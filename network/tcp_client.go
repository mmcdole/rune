package network

import (
	"net"
	"sync"
	"time"

	"github.com/drake/rune/mud"
)

// TCPClient manages the lifecycle of TCP connections.
// It provides a stable interface for the Session while handling the
// chaotic reality of network sockets underneath.
type TCPClient struct {
	// Stable channel that Session reads from. Never closes.
	outputChan chan mud.Event

	// State protection
	mu      sync.Mutex
	current *connection // The currently active connection, or nil
}

// connection represents a single, ephemeral TCP session.
// It is created on Connect() and discarded on Disconnect().
type connection struct {
	conn      net.Conn
	telnet    *TelnetBuffer
	recvQueue chan mud.Event

	// Buffered queue for outgoing data specific to this connection
	sendQueue chan string

	// Signal to stop internal goroutines (Writer)
	done      chan struct{}
	closeOnce sync.Once
}

// NewTCPClient creates a new client.
func NewTCPClient() *TCPClient {
	return &TCPClient{
		outputChan: make(chan mud.Event, 4096),
	}
}

// Connect establishes a new connection.
// If a connection already exists, it is cleanly closed and replaced.
func (c *TCPClient) Connect(address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Clean up existing connection if present
	if c.current != nil {
		c.current.close()
	}

	// 2. Dial new connection
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return err
	}

	// 3. Configure TCP KeepAlive (for detecting dropped connections)
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	// 4. Create the new connection object
	cx := &connection{
		conn:      conn,
		telnet:    NewTelnetBuffer(),
		sendQueue: make(chan string, 1024),
		recvQueue: make(chan mud.Event, 4096),
		done:      make(chan struct{}),
	}

	// 5. Set as current and start workers
	c.current = cx

	// We pass the specific 'cx' pointer to the workers.
	// They bind to THIS connection instance, not the TCPClient.
	go c.readLoop(cx)
	go c.writeLoop(cx)
	go c.drainLoop(cx)

	return nil
}

// Disconnect manually closes the connection.
func (c *TCPClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.current != nil {
		c.current.close()
		c.current = nil
		// We initiate the disconnect, so we don't send an event.
		// The readLoop will see the close, check c.current, see it's nil, and exit silently.
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
			// Drop rather than block callers
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
func (c *TCPClient) readLoop(cx *connection) {
	buf := make([]byte, 4096)

	for {
		// BLOCKING READ.
		// Exits on: Network Error, Remote Close, or Local Close.
		n, err := cx.conn.Read(buf)
		if err != nil {
			// THE CRITICAL CHECK:
			// Why did we fail? Was it a crash, or were we replaced?
			c.mu.Lock()
			isCurrent := (c.current == cx)
			if isCurrent {
				// We are the active connection and we just died.
				// This is a real disconnect.
				c.current = nil
			}
			c.mu.Unlock()

			if isCurrent {
				cx.enqueueEvent(mud.Event{
					Type:    mud.EventSystemControl,
					Control: mud.ControlOp{Action: mud.ActionDisconnect},
				})
				cx.shutdown()
			}
			return
		}

		if n == 0 {
			continue
		}

		// Protocol processing
		responses := cx.telnet.ProcessBytes(buf[:n])
		if len(responses) > 0 {
			// Best effort write for negotiation
			cx.conn.SetWriteDeadline(time.Now().Add(time.Second))
			cx.conn.Write(responses)
			cx.conn.SetWriteDeadline(time.Time{})
		}

		// Emit Lines
		for _, line := range cx.telnet.ExtractLines() {
			cx.enqueueEvent(mud.Event{
				Type:    mud.EventNetLine,
				Payload: line,
			})
		}

		// Emit Prompts
		pending := cx.telnet.GetPending(false)
		if len(pending) > 0 {
			payload := pending
			if cx.telnet.HasSignal() {
				payload = cx.telnet.GetPending(true) // consume when signal seen
			} else {
				// consume even without explicit signal to avoid prompt accumulation
				cx.telnet.GetPending(true)
			}
			cx.enqueueEvent(mud.Event{
				Type:    mud.EventNetPrompt,
				Payload: payload,
			})
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
			// Write with deadline to prevent hanging on stalled connections
			cx.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err := cx.conn.Write([]byte(data + "\r\n"))
			cx.conn.SetWriteDeadline(time.Time{})

			if err != nil {
				// We don't need to handle cleanup here.
				// The Read() in readLoop will fail momentarily and handle it.
				return
			}
		}
	}
}

// drainLoop forwards queued events to the stable output channel. It is allowed to
// block on outputChan, but the TCP reader remains unblocked because it only
// writes to the queue.
func (c *TCPClient) drainLoop(cx *connection) {
	for {
		select {
		case <-cx.done:
			return
		case item := <-cx.recvQueue:
			c.outputChan <- item
		}
	}
}

// close cleanly shuts down the connection resources
func (cx *connection) close() {
	// Closing the net.Conn causes Read() to return error immediately
	cx.conn.Close()
	cx.shutdown()
}

// enqueueEvent is a best-effort send that drops on full buffers to keep the reader moving.
func (cx *connection) enqueueEvent(evt mud.Event) {
	select {
	case <-cx.done:
		return
	case cx.recvQueue <- evt:
	default:
		// Drop rather than block the TCP reader
	}
}

// shutdown closes the done channel exactly once to stop workers.
func (cx *connection) shutdown() {
	cx.closeOnce.Do(func() {
		close(cx.done)
	})
}
