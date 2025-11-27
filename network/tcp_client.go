package network

import (
	"net"
	"sync"
	"time"

	"github.com/drake/rune/mud"
)

// TCPClient implements the Network interface with real telnet support
type TCPClient struct {
	conn       net.Conn
	outputChan chan mud.Event
	sendChan   chan string
	telnet     *TelnetBuffer

	mu        sync.Mutex
	connected bool
	done      chan struct{}
}

// NewTCPClient initializes a TCP client with buffered channels for async I/O.
func NewTCPClient() *TCPClient {
	return &TCPClient{
		outputChan: make(chan mud.Event, 256),
		sendChan:   make(chan string, 64),
		telnet:     NewTelnetBuffer(),
	}
}

// Connect establishes a connection to the MUD server
func (c *TCPClient) Connect(address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		c.disconnectLocked()
	}

	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return err
	}

	c.conn = conn
	c.connected = true
	c.done = make(chan struct{})
	c.telnet.Clear()

	// Start reader and writer goroutines
	go c.readerLoop()
	go c.writerLoop()

	return nil
}

// Disconnect closes the connection
func (c *TCPClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disconnectLocked()
}

func (c *TCPClient) disconnectLocked() {
	if !c.connected {
		return
	}

	c.connected = false
	close(c.done)

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// Send queues data to be sent to the server
func (c *TCPClient) Send(data string) {
	select {
	case c.sendChan <- data:
	default:
		// Channel full, drop data (or could block)
	}
}

// Output returns the channel of events from the server
func (c *TCPClient) Output() <-chan mud.Event {
	return c.outputChan
}

// readerLoop reads from the connection and processes telnet protocol
// Uses reactive prompt detection - no timeout-based polling
func (c *TCPClient) readerLoop() {
	buf := make([]byte, 4096)

	for {
		// Blocking read - no timeout
		n, err := c.conn.Read(buf)
		if err != nil {
			// Connection closed or error - properly cleanup to signal writerLoop
			c.mu.Lock()
			if c.connected {
				c.connected = false
				close(c.done)
			}
			c.mu.Unlock()
			return
		}

		if n == 0 {
			continue
		}

		// Process telnet protocol
		responses := c.telnet.ProcessBytes(buf[:n])

		// Send any negotiation responses back to server
		if len(responses) > 0 {
			c.conn.Write(responses)
		}

		// Extract and emit complete lines
		lines := c.telnet.ExtractLines()
		for _, line := range lines {
			c.outputChan <- mud.Event{
				Type:    mud.EventServerLine,
				Payload: line,
			}
		}

		// Reactive prompt detection - only check when data arrives
		pending := c.telnet.GetPending(false) // Peek only

		if len(pending) > 0 && len(pending) < 500 {
			if c.telnet.HasSignal() {
				// Terminated Prompt (Server sent GA/EOR)
				// Consume the buffer and emit as prompt
				finalPrompt := c.telnet.GetPending(true)
				c.outputChan <- mud.Event{
					Type:    mud.EventServerPrompt,
					Payload: finalPrompt,
				}
			} else {
				// Unterminated Prompt (No signal)
				// Emit on data arrival - let UI handle deduplication
				c.outputChan <- mud.Event{
					Type:    mud.EventServerPrompt,
					Payload: pending,
				}
			}
		}
	}
}

// writerLoop sends data to the connection
func (c *TCPClient) writerLoop() {
	for {
		select {
		case <-c.done:
			return
		case data := <-c.sendChan:
			c.mu.Lock()
			if c.connected && c.conn != nil {
				// Add CR+LF as per telnet protocol
				c.conn.Write([]byte(data + "\r\n"))
			}
			c.mu.Unlock()
		}
	}
}

// IsConnected returns the connection status
func (c *TCPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}
