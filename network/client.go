package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TCPClient manages the lifecycle of TCP connections.
// It provides a stable interface for the Session while handling the
// chaotic reality of network sockets underneath.
type TCPClient struct {
	// Stable channel that Session reads from. Never closes.
	// Small buffer allows TCP backpressure to work naturally.
	outputChan chan Output

	// State protection
	mu      sync.Mutex
	current *connection // The currently active connection, or nil
}

// outMsg is a queued write. line messages are user commands (CRLF
// appended, prompt buffer cleared); raw messages are protocol bytes
// such as telnet negotiation replies, written verbatim.
type outMsg struct {
	data []byte
	line bool
}

// connection represents a single, ephemeral TCP session.
// It is created on Connect() and discarded on Disconnect().
type connection struct {
	conn   net.Conn
	parser *Parser
	output *OutputBuffer

	// Telnet mode tracking - separate flags allow proper reversion
	willEOR atomic.Bool // Server indicated WILL EOR
	willSGA atomic.Bool // Server indicated WILL SGA (Suppress Go Ahead)

	localEcho atomic.Bool

	// Buffered queue for outgoing data specific to this connection.
	// writeLoop is the ONLY goroutine that writes to conn (and the only
	// one that touches write deadlines); everything else enqueues here.
	sendQueue chan outMsg

	// Signal to stop internal goroutines
	done      chan struct{}
	closeOnce sync.Once
}

// NewTCPClient creates a new client.
func NewTCPClient() *TCPClient {
	return &TCPClient{
		// Small buffer - let TCP backpressure handle flow control
		outputChan: make(chan Output, 256),
	}
}

// splitAddress separates an optional scheme prefix from host:port.
// Supported schemes: "telnet://" (plain TCP, the default when no
// scheme is given), "tls://" (TLS with certificate verification), and
// "tls+insecure://" (TLS without verification - many MUDs run
// self-signed certificates).
func splitAddress(address string) (hostport string, useTLS, insecure bool, err error) {
	scheme, rest, found := strings.Cut(address, "://")
	if !found {
		return address, false, false, nil
	}
	switch scheme {
	case "telnet", "tcp":
		return rest, false, false, nil
	case "tls":
		return rest, true, false, nil
	case "tls+insecure":
		return rest, true, true, nil
	default:
		return "", false, false, fmt.Errorf("unknown scheme %q (use telnet://, tls:// or tls+insecure://)", scheme)
	}
}

// Connect establishes a new connection.
// If a connection already exists, it is cleanly closed and replaced.
func (c *TCPClient) Connect(ctx context.Context, address string) error {
	hostport, useTLS, insecure, err := splitAddress(address)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up existing connection if present
	if c.current != nil {
		c.current.close()
	}

	// Dial with context to respect app shutdown during connection attempts
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", hostport)
	if err != nil {
		return err
	}

	// Configure TCP KeepAlive (for detecting dropped connections)
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	if useTLS {
		host, _, splitErr := net.SplitHostPort(hostport)
		if splitErr != nil {
			host = hostport
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecure,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return fmt.Errorf("TLS handshake: %w", err)
		}
		conn = tlsConn
	}

	// Create the new connection object
	cx := &connection{
		conn:      conn,
		parser:    NewParser(defaultCompatibility()),
		output:    NewOutputBuffer(TelnetModeUnterminated),
		sendQueue: make(chan outMsg, 4096),
		done:      make(chan struct{}),
	}
	cx.localEcho.Store(true)
	// willEOR and willSGA default to false (unterminated prompt mode)

	// Set as current and start workers
	c.current = cx
	go c.readLoop(cx)
	go c.writeLoop(cx)

	return nil
}

// Disconnect manually closes the connection.
func (c *TCPClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.current != nil {
		c.current.close()
		c.current = nil
	}
}

// Send queues data for the current connection.
// Returns error immediately if not connected or buffer is full.
func (c *TCPClient) Send(data string) error {
	c.mu.Lock()
	cx := c.current
	c.mu.Unlock()

	if cx == nil {
		return fmt.Errorf("not connected")
	}

	select {
	case cx.sendQueue <- outMsg{data: []byte(data), line: true}:
		return nil
	default:
		return fmt.Errorf("send buffer full (network stalled?)")
	}
}

// Output returns the stable output channel.
func (c *TCPClient) Output() <-chan Output {
	return c.outputChan
}

// LocalEchoEnabled reports whether the current connection prefers local echo.
// Defaults to true if no active connection.
func (c *TCPClient) LocalEchoEnabled() bool {
	c.mu.Lock()
	cx := c.current
	c.mu.Unlock()
	if cx == nil {
		return true
	}
	return cx.localEcho.Load()
}

// --- Worker Routines ---

// readLoop reads from a specific connection instance.
// It sends directly to outputChan, blocking if the session is slow.
// This allows TCP backpressure to naturally throttle the server.
func (c *TCPClient) readLoop(cx *connection) {
	buf := make([]byte, 4096)

	for {
		n, err := cx.conn.Read(buf)
		if err != nil {
			// Check if we're still the active connection
			c.mu.Lock()
			isCurrent := (c.current == cx)
			if isCurrent {
				c.current = nil
			}
			c.mu.Unlock()

			if isCurrent {
				// Send disconnect notification - this may block briefly, that's OK
				select {
				case c.outputChan <- Output{Kind: OutputDisconnect}:
				case <-cx.done:
				}
				cx.shutdown()
			}
			return
		}

		if n == 0 {
			continue
		}

		for _, ev := range cx.parser.Receive(buf[:n]) {
			switch ev.Kind {
			case TelnetEventDataSend:
				// Negotiation replies go through the send queue so a
				// single goroutine owns the socket writes and their
				// deadlines. Blocking here is fine: writeLoop drains
				// continuously, and done unblocks us on teardown.
				select {
				case cx.sendQueue <- outMsg{data: ev.Data}:
				case <-cx.done:
					return
				}

			case TelnetEventDataReceive:
				lines := cx.output.Receive(ev.Data)
				for _, line := range lines {
					select {
					case c.outputChan <- Output{Kind: OutputLine, Payload: string(line)}:
					case <-cx.done:
						return
					}
				}
				if cx.telnetMode() == TelnetModeUnterminated {
					prompt := cx.output.Prompt(false)
					if prompt != "" {
						select {
						case c.outputChan <- Output{Kind: OutputPrompt, Payload: prompt}:
						case <-cx.done:
							return
						}
					}
				}

			case TelnetEventIAC:
				if ev.Command == CmdGA || ev.Command == CmdEOR {
					// GA/EOR commands indicate prompt termination for this message,
					// but don't change the negotiated mode (that's done via WILL/WONT)
					if cx.output.HasNewData() {
						prompt := cx.output.Prompt(true)
						if prompt != "" {
							select {
							case c.outputChan <- Output{Kind: OutputPrompt, Payload: prompt}:
							case <-cx.done:
								return
							}
						}
					} else {
						// Just flush even if no new data
						cx.output.Prompt(true)
					}
				}

			case TelnetEventNegotiation:
				cx.applyNegotiation(ev.Command, ev.Option)

			case TelnetEventSubnegotiation:
				// No-op for now; surface via prompt when applicable

			case TelnetEventDecompressImmediate:
				// Compression not supported yet; ignore payload for now
			}
		}
	}
}

// writeLoop handles outgoing data for a specific connection.
// It is the sole writer to the socket, so write deadlines cannot race.
func (c *TCPClient) writeLoop(cx *connection) {
	for {
		select {
		case <-cx.done:
			return
		case msg := <-cx.sendQueue:
			data := msg.data
			if msg.line {
				// Clear prompt buffer before sending - in unterminated mode,
				// the server will reprint the prompt after echoing our input
				cx.output.InputSent()
				data = append(data, '\r', '\n')
			}

			cx.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err := cx.conn.Write(data)
			cx.conn.SetWriteDeadline(time.Time{})

			if err != nil {
				// Write failed - close the connection to trigger readLoop cleanup
				cx.conn.Close()
				return
			}
		}
	}
}

// close cleanly shuts down the connection resources
func (cx *connection) close() {
	cx.conn.Close()
	cx.shutdown()
}

// shutdown closes the done channel exactly once to stop workers.
func (cx *connection) shutdown() {
	cx.closeOnce.Do(func() {
		close(cx.done)
	})
}

// applyNegotiation updates local state based on telnet negotiation events.
func (cx *connection) applyNegotiation(cmd, opt byte) {
	switch opt {
	case OptEcho:
		switch cmd {
		case CmdWILL:
			// Server will echo - disable local echo
			cx.localEcho.Store(false)
		case CmdWONT, CmdDONT, CmdDO:
			// Server won't echo or wants us to echo - enable local echo
			cx.localEcho.Store(true)
		}
	case OptEOR:
		switch cmd {
		case CmdWILL, CmdDO:
			cx.willEOR.Store(true)
			cx.updateTelnetMode()
		case CmdWONT, CmdDONT:
			cx.willEOR.Store(false)
			cx.updateTelnetMode()
		}
	case OptSGA:
		switch cmd {
		case CmdWILL, CmdDO:
			cx.willSGA.Store(true)
			cx.updateTelnetMode()
		case CmdWONT, CmdDONT:
			cx.willSGA.Store(false)
			cx.updateTelnetMode()
		}
	}
}

// telnetMode returns the current telnet mode based on negotiation state.
func (cx *connection) telnetMode() TelnetMode {
	if cx.willEOR.Load() || cx.willSGA.Load() {
		return TelnetModeTerminatedPrompt
	}
	return TelnetModeUnterminated
}

// updateTelnetMode recalculates and applies the telnet mode to the output buffer.
func (cx *connection) updateTelnetMode() {
	cx.output.SetMode(cx.telnetMode())
}
