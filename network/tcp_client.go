package network

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drake/rune/mud"
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
	outputChan chan mud.Event

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
	telnet *TelnetBuffer

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
		outputChan: make(chan mud.Event, 256),
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
func (c *TCPClient) Connect(address string) error {
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

	// Dial new connection
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
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
		telnet:    NewTelnetBuffer(),
		sendQueue: make(chan string, 256),
		done:      make(chan struct{}),
	}

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
// Safe to call even if disconnected (data is dropped).
func (c *TCPClient) Send(data string) {
	c.mu.Lock()
	cx := c.current
	c.mu.Unlock()

	if cx != nil {
		select {
		case cx.sendQueue <- data:
		default:
			// Queue full - drop to avoid blocking Lua
		}
	}
}

// Output returns the stable event channel.
func (c *TCPClient) Output() <-chan mud.Event {
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
	return cx.telnet.LocalEchoEnabled()
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
				case c.outputChan <- mud.Event{
					Type:    mud.EventSystemControl,
					Control: mud.ControlOp{Action: mud.ActionDisconnect},
				}:
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

		// Protocol processing
		responses := cx.telnet.ProcessBytes(buf[:n])
		if len(responses) > 0 {
			cx.conn.SetWriteDeadline(time.Now().Add(time.Second))
			cx.conn.Write(responses)
			cx.conn.SetWriteDeadline(time.Time{})
		}

		// Emit Lines - blocking send to outputChan
		for _, line := range cx.telnet.ExtractLines() {
			c.linesEmitted.Add(1)
			select {
			case c.outputChan <- mud.Event{Type: mud.EventNetLine, Payload: line}:
			case <-cx.done:
				return
			}
		}

		// Emit Prompts
		pending := cx.telnet.GetPending(false)
		if len(pending) > 0 {
			payload := pending
			if cx.telnet.HasSignal() {
				payload = cx.telnet.GetPending(true)
			} else {
				cx.telnet.GetPending(true)
			}
			select {
			case c.outputChan <- mud.Event{Type: mud.EventNetPrompt, Payload: payload}:
			case <-cx.done:
				return
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
			cx.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			n, err := cx.conn.Write([]byte(data + "\r\n"))
			cx.conn.SetWriteDeadline(time.Time{})

			if err != nil {
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
