package network

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/tls"
	"fmt"
	"io"
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

	// Last known window size, retained across connections so NAWS
	// can answer immediately on the next connect.
	width, height int
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

	// Read source indirection for MCCP2. reader is what readLoop
	// consumes: the socket normally, a zlib stream while compression
	// is active. raw is the underlying byte source compression wraps -
	// a byte-exact bufio.Reader once compression has run, so a clean
	// zlib stream end resumes plain telnet without losing bytes.
	// Only readLoop touches these after Connect returns.
	reader     io.Reader
	raw        io.Reader
	zr         io.ReadCloser
	compressed bool

	// Identity negotiation responder (TTYPE/MTTS, NAWS, CHARSET, MNES)
	hs *handshake

	// Telnet mode tracking - separate flags allow proper reversion
	willEOR atomic.Bool // Server indicated WILL EOR
	willSGA atomic.Bool // Server indicated WILL SGA (Suppress Go Ahead)

	gmcpActive atomic.Bool // GMCP negotiated on this connection

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
		reader:    conn,
		raw:       conn,
		hs:        newHandshake(useTLS, c.width, c.height),
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

// SetWindowSize records the terminal size and, when NAWS is active on
// the current connection, reports it to the server immediately.
// The size is retained across connections so the next connect can
// answer DO NAWS with real numbers.
func (c *TCPClient) SetWindowSize(width, height int) {
	c.mu.Lock()
	c.width, c.height = width, height
	cx := c.current
	c.mu.Unlock()

	if cx == nil {
		return
	}
	frame := cx.hs.setWindowSize(width, height)
	if frame == nil {
		return
	}
	select {
	case cx.sendQueue <- outMsg{data: frame}:
	default:
		// Send queue full - drop the resize report; the next resize
		// (or reconnect) will correct it. Never block the UI path.
	}
}

// SendGMCP sends a GMCP message: "Package.SubPackage" plus optional
// raw JSON. Returns an error when disconnected or when the server has
// not negotiated GMCP.
func (c *TCPClient) SendGMCP(pkg, data string) error {
	c.mu.Lock()
	cx := c.current
	c.mu.Unlock()

	if cx == nil {
		return fmt.Errorf("not connected")
	}
	if !cx.gmcpActive.Load() {
		return fmt.Errorf("GMCP not negotiated on this connection")
	}

	payload := pkg
	if data != "" {
		payload += " " + data
	}
	frame := subnegFrame(OptGMCP, []byte(payload))

	select {
	case cx.sendQueue <- outMsg{data: frame}:
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
		n, err := cx.reader.Read(buf)

		if n > 0 && !c.processIncoming(cx, buf[:n]) {
			return
		}

		if err != nil {
			// A clean zlib stream end is not a connection error: MCCP
			// may terminate and the server resumes plain telnet. The
			// byte-exact raw reader still holds any bytes that
			// followed the compressed stream.
			if cx.compressed && err == io.EOF {
				cx.zr.Close()
				cx.reader = cx.raw
				cx.compressed = false
				continue
			}

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
	}
}

// processIncoming feeds bytes to the parser and dispatches the
// resulting events. Returns false when the connection is done and the
// read loop should exit.
func (c *TCPClient) processIncoming(cx *connection, data []byte) bool {
	// MCCP2 activation is deferred to the end of the batch: the parser
	// stops parsing at IAC SB 86 IAC SE and hands back the remaining
	// raw (compressed) bytes, so nothing after the marker is parsed.
	startMCCP := false
	var mccpRest []byte

	for _, ev := range cx.parser.Receive(data) {
		switch ev.Kind {
		case TelnetEventDataSend:
			// Negotiation replies go through the send queue so a
			// single goroutine owns the socket writes and their
			// deadlines. Blocking here is fine: writeLoop drains
			// continuously, and done unblocks us on teardown.
			if !cx.enqueueRaw(ev.Data) {
				return false
			}

		case TelnetEventDataReceive:
			lines := cx.output.Receive(ev.Data)
			for _, line := range lines {
				select {
				case c.outputChan <- Output{Kind: OutputLine, Payload: string(line)}:
				case <-cx.done:
					return false
				}
			}
			if cx.telnetMode() == TelnetModeUnterminated {
				prompt := cx.output.Prompt(false)
				if prompt != "" {
					select {
					case c.outputChan <- Output{Kind: OutputPrompt, Payload: prompt}:
					case <-cx.done:
						return false
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
							return false
						}
					}
				} else {
					// Just flush even if no new data
					cx.output.Prompt(true)
				}
			}

		case TelnetEventNegotiation:
			cx.applyNegotiation(ev.Command, ev.Option)
			for _, frame := range cx.hs.onNegotiation(ev.Command, ev.Option) {
				if !cx.enqueueRaw(frame) {
					return false
				}
			}
			if ev.Option == OptGMCP {
				switch ev.Command {
				case CmdWILL, CmdDO:
					if !cx.gmcpActive.Swap(true) {
						select {
						case c.outputChan <- Output{Kind: OutputGMCPEnabled}:
						case <-cx.done:
							return false
						}
					}
				case CmdWONT, CmdDONT:
					cx.gmcpActive.Store(false)
				}
			}

		case TelnetEventSubnegotiation:
			switch ev.Option {
			case OptMCCP2:
				startMCCP = true
			case OptGMCP:
				pkg, payload := splitGMCP(ev.Data)
				if pkg != "" {
					select {
					case c.outputChan <- Output{Kind: OutputGMCP, Package: pkg, Payload: payload}:
					case <-cx.done:
						return false
					}
				}
			default:
				for _, frame := range cx.hs.onSubnegotiation(ev.Option, ev.Data) {
					if !cx.enqueueRaw(frame) {
						return false
					}
				}
			}

		case TelnetEventDecompressImmediate:
			// Raw compressed bytes that followed IAC SB 86 IAC SE in
			// the same read. Always the final event of a batch.
			startMCCP = true
			mccpRest = ev.Data
		}
	}

	if startMCCP {
		if err := cx.startDecompression(mccpRest); err != nil {
			// The stream is unrecoverable without valid zlib data -
			// close the socket; readLoop's error path reports the
			// disconnect.
			cx.conn.Close()
			return true // let readLoop observe the read error
		}
	}
	return true
}

// enqueueRaw queues protocol bytes for writeLoop. Returns false if the
// connection is shutting down.
func (cx *connection) enqueueRaw(data []byte) bool {
	select {
	case cx.sendQueue <- outMsg{data: data}:
		return true
	case <-cx.done:
		return false
	}
}

// startDecompression switches the read path to a zlib stream, seeded
// with any compressed bytes that arrived in the activating read. The
// underlying source becomes a byte-exact bufio.Reader, so zlib never
// over-reads and a clean stream end can resume plain telnet.
func (cx *connection) startDecompression(remaining []byte) error {
	if cx.compressed {
		return nil
	}
	src := bufio.NewReader(io.MultiReader(bytes.NewReader(remaining), cx.raw))
	zr, err := zlib.NewReader(src)
	if err != nil {
		return err
	}
	cx.raw = src
	cx.zr = zr
	cx.reader = zr
	cx.compressed = true
	return nil
}

// splitGMCP separates "Package.SubPackage <json>" into the package
// name and the raw JSON payload (which may be empty).
func splitGMCP(data []byte) (pkg, payload string) {
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		return "", ""
	}
	if i := strings.IndexByte(msg, ' '); i >= 0 {
		return msg[:i], strings.TrimSpace(msg[i+1:])
	}
	return msg, ""
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
