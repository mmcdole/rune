package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drake/rune/event"
)

// Stats holds network statistics for monitoring.
type Stats struct {
	Connected      bool
	BytesRead      uint64
	BytesWritten   uint64
	LinesEmitted   uint64
	LastReadTime   time.Time
	SendQueueLen   int
	SendQueueCap   int
	OutputQueueLen int
	OutputQueueCap int
}

// TCPClient manages the lifecycle of TCP connections.
// It provides a stable interface for the Session while handling the
// chaotic reality of network sockets underneath.
type TCPClient struct {
	// Stable channel that Session reads from. Never closes.
	// Small buffer allows TCP backpressure to work naturally.
	outputChan chan event.Event

	// State protection
	mu      sync.Mutex
	current *connection // The currently active connection, or nil

	// Stats (atomic for lock-free reads)
	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
	linesEmitted atomic.Uint64
	lastReadTime atomic.Int64 // Unix nano
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

	// Buffered queue for outgoing data specific to this connection
	sendQueue chan string

	// Signal to stop internal goroutines
	done      chan struct{}
	closeOnce sync.Once
}

// NewTCPClient creates a new client.
func NewTCPClient() *TCPClient {
	return &TCPClient{
		// Small buffer - let TCP backpressure handle flow control
		outputChan: make(chan event.Event, 256),
	}
}

// Stats returns current network statistics.
func (c *TCPClient) Stats() Stats {
	c.mu.Lock()
	cx := c.current
	var sendQLen, sendQCap int
	if cx != nil {
		sendQLen = len(cx.sendQueue)
		sendQCap = cap(cx.sendQueue)
	}
	c.mu.Unlock()

	lastRead := time.Unix(0, c.lastReadTime.Load())
	if lastRead.Unix() == 0 {
		lastRead = time.Time{}
	}

	return Stats{
		Connected:      cx != nil,
		BytesRead:      c.bytesRead.Load(),
		BytesWritten:   c.bytesWritten.Load(),
		LinesEmitted:   c.linesEmitted.Load(),
		LastReadTime:   lastRead,
		SendQueueLen:   sendQLen,
		SendQueueCap:   sendQCap,
		OutputQueueLen: len(c.outputChan),
		OutputQueueCap: cap(c.outputChan),
	}
}

// Connect establishes a new connection.
// If a connection already exists, it is cleanly closed and replaced.
func (c *TCPClient) Connect(ctx context.Context, address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up existing connection if present
	if c.current != nil {
		c.current.close()
	}

	// Reset stats for new connection
	c.bytesRead.Store(0)
	c.bytesWritten.Store(0)
	c.linesEmitted.Store(0)
	c.lastReadTime.Store(0)

	// Dial with context to respect app shutdown during connection attempts
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}

	// Configure TCP KeepAlive (for detecting dropped connections)
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	// Create the new connection object
	cx := &connection{
		conn:      conn,
		parser:    NewParser(defaultCompatibility()),
		output:    NewOutputBuffer(TelnetModeUnterminated),
		sendQueue: make(chan string, 4096),
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
	case cx.sendQueue <- data:
		return nil
	default:
		return fmt.Errorf("send buffer full (network stalled?)")
	}
}

// Output returns the stable event channel.
func (c *TCPClient) Output() <-chan event.Event {
	return c.outputChan
}

// IsConnected checks if there is an active connection.
func (c *TCPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current != nil
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
				// Send disconnect event - this may block briefly, that's OK
				select {
				case c.outputChan <- event.Event{Type: event.SysDisconnect}:
				case <-cx.done:
				}
				cx.shutdown()
			}
			return
		}

		if n == 0 {
			continue
		}

		// Update stats
		c.bytesRead.Add(uint64(n))
		c.lastReadTime.Store(time.Now().UnixNano())

		for _, ev := range cx.parser.Receive(buf[:n]) {
			switch ev.Kind {
			case TelnetEventDataSend:
				cx.conn.SetWriteDeadline(time.Now().Add(time.Second))
				written, werr := cx.conn.Write(ev.Data)
				cx.conn.SetWriteDeadline(time.Time{})
				if werr != nil {
					return
				}
				c.bytesWritten.Add(uint64(written))

			case TelnetEventDataReceive:
				lines := cx.output.Receive(ev.Data)
				for _, line := range lines {
					c.linesEmitted.Add(1)
					select {
					case c.outputChan <- event.Event{Type: event.NetLine, Payload: string(line)}:
					case <-cx.done:
						return
					}
				}
				if cx.telnetMode() == TelnetModeUnterminated {
					prompt := cx.output.Prompt(false)
					if prompt != "" {
						select {
						case c.outputChan <- event.Event{Type: event.NetPrompt, Payload: prompt}:
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
							case c.outputChan <- event.Event{Type: event.NetPrompt, Payload: prompt}:
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
func (c *TCPClient) writeLoop(cx *connection) {
	for {
		select {
		case <-cx.done:
			return
		case data := <-cx.sendQueue:
			// Clear prompt buffer before sending - in unterminated mode,
			// the server will reprint the prompt after echoing our input
			cx.output.InputSent()

			cx.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			n, err := cx.conn.Write([]byte(data + "\r\n"))
			cx.conn.SetWriteDeadline(time.Time{})

			if err != nil {
				// Write failed - close the connection to trigger readLoop cleanup
				cx.conn.Close()
				return
			}
			c.bytesWritten.Add(uint64(n))
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
