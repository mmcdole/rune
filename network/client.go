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
	"os"
	"strings"
	"sync"
	"time"
)

// TCPClient owns the active TCP connection and exposes owned parser batches.
type TCPClient struct {
	// Stable across connections; blocking publication applies TCP backpressure.
	inboundChan chan Inbound

	mu                  sync.Mutex
	current             *connection
	desiredConnectionID uint64 // Latest connection ID requested by Session
}

// outMsg is either a game line or raw Telnet protocol data.
type outMsg struct {
	data []byte
	line bool
}

// connection owns the state and workers for one TCP connection.
type connection struct {
	conn         net.Conn
	connectionID uint64
	parser       *Parser

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
	secure     bool

	// writeLoop is the sole socket writer; other goroutines enqueue here.
	sendQueue chan outMsg

	// Signal to stop internal goroutines
	done      chan struct{}
	closeOnce sync.Once
}

// NewTCPClient creates a new client.
func NewTCPClient() *TCPClient {
	return &TCPClient{
		inboundChan: make(chan Inbound, 256),
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

// dialOverride maps a canonical host:port to an alternate dial target via
// RUNE_DIAL_OVERRIDES, a comma-separated list of canonical=actual pairs
// (e.g. "mud.example.com:4000=127.0.0.1:4000"). The override changes only
// where the TCP connection goes: the canonical address remains the
// connection's identity for display, reconnect state, and TLS SNI. It is a
// development seam - demo recordings and tests stand up a local scripted
// server under a real-looking address - and is unset in normal use.
func dialOverride(hostport string) string {
	overrides := os.Getenv("RUNE_DIAL_OVERRIDES")
	if overrides == "" {
		return hostport
	}
	for _, pair := range strings.Split(overrides, ",") {
		canonical, actual, found := strings.Cut(strings.TrimSpace(pair), "=")
		if found && canonical == hostport {
			return actual
		}
	}
	return hostport
}

// Connect dials address and installs it unless connectionID was superseded.
func (c *TCPClient) Connect(ctx context.Context, address string, connectionID uint64) error {
	hostport, useTLS, insecure, err := splitAddress(address)
	if err != nil {
		return err
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", dialOverride(hostport))
	if err != nil {
		return err
	}

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
	cx := &connection{
		conn:         conn,
		connectionID: connectionID,
		reader:       conn,
		raw:          conn,
		parser:       NewParser(defaultCompatibility()),
		sendQueue:    make(chan outMsg, 4096),
		done:         make(chan struct{}),
		secure:       useTLS,
	}

	// Ignore a dial superseded by BeginConnect or Disconnect.
	c.mu.Lock()
	if c.desiredConnectionID != connectionID {
		c.mu.Unlock()
		conn.Close()
		return context.Canceled
	}
	old := c.current
	c.current = cx
	c.mu.Unlock()
	if old != nil {
		old.close()
	}
	go c.readLoop(cx)
	go c.writeLoop(cx)

	return nil
}

// BeginConnect records the next connection ID before its asynchronous dial,
// so older dials cannot replace it.
func (c *TCPClient) BeginConnect(connectionID uint64) {
	c.mu.Lock()
	c.desiredConnectionID = connectionID
	old := c.current
	c.current = nil
	c.mu.Unlock()
	if old != nil {
		old.close()
	}
}

// Disconnect rejects pending dials and closes the active connection.
func (c *TCPClient) Disconnect() {
	c.mu.Lock()
	c.desiredConnectionID++ // invalidate pending dials
	old := c.current
	c.current = nil
	c.mu.Unlock()
	if old != nil {
		old.close()
	}
}

// SendLine queues one game line for the identified connection. Validation and
// enqueue happen under the same lock, so a reconnect cannot redirect stale
// Session work to the new socket.
func (c *TCPClient) SendLine(connectionID uint64, line string) error {
	if strings.ContainsAny(line, "\r\n") {
		return fmt.Errorf("game line cannot contain CR or LF")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cx := c.current
	if cx == nil || cx.connectionID != connectionID {
		return fmt.Errorf("not connected")
	}

	select {
	case cx.sendQueue <- outMsg{data: []byte(line), line: true}:
		return nil
	case <-cx.done:
		return fmt.Errorf("not connected")
	default:
		return fmt.Errorf("send buffer full (network stalled?)")
	}
}

// SendFrame queues raw Telnet protocol bytes for the identified connection.
func (c *TCPClient) SendFrame(connectionID uint64, frame []byte) error {
	owned := bytes.Clone(frame)
	c.mu.Lock()
	defer c.mu.Unlock()
	cx := c.current
	if cx == nil || cx.connectionID != connectionID {
		return fmt.Errorf("not connected")
	}
	select {
	case cx.sendQueue <- outMsg{data: owned}:
		return nil
	case <-cx.done:
		return fmt.Errorf("not connected")
	default:
		return fmt.Errorf("send buffer full (network stalled?)")
	}
}

// Inbound returns parser batches and disconnects from all connections. Each
// message carries its connection ID so Session can reject stale work.
func (c *TCPClient) Inbound() <-chan Inbound {
	return c.inboundChan
}

func (c *TCPClient) publish(cx *connection, inbound Inbound) bool {
	inbound.ConnectionID = cx.connectionID
	select {
	case c.inboundChan <- inbound:
		return true
	case <-cx.done:
		return false
	}
}

// readLoop reads cx and blocks on inbound publication to apply TCP backpressure.
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

			c.mu.Lock()
			isCurrent := (c.current == cx)
			if isCurrent {
				c.current = nil
			}
			c.mu.Unlock()

			if isCurrent {
				c.publish(cx, Inbound{Kind: InboundDisconnect})
				cx.shutdown()
			}
			return
		}
	}
}

// processIncoming consumes transport-local MCCP activation, then publishes any
// remaining events from this parser call as one owned batch. MCCP activation
// changes how the next socket bytes must be read; all Session-facing events
// cross the boundary in their original order.
func (c *TCPClient) processIncoming(cx *connection, data []byte) bool {
	events := cx.parser.Receive(data)
	if len(events) == 0 {
		return true
	}

	forwarded := make([]TelnetEvent, 0, len(events))
	startMCCP := false
	var mccpRest []byte
	for _, event := range events {
		switch {
		case event.Kind == TelnetEventSubnegotiation && (event.Option == OptMCCP2 || event.Option == OptMCCP3):
			startMCCP = true
		case event.Kind == TelnetEventDecompressImmediate:
			startMCCP = true
			mccpRest = event.Data
		default:
			forwarded = append(forwarded, event)
		}
	}

	if len(forwarded) > 0 && !c.publish(cx, Inbound{
		Kind: InboundEvents,
		Batch: EventBatch{
			Secure: cx.secure,
			Events: cloneEvents(forwarded),
		},
	}) {
		return false
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

func cloneEvents(events []TelnetEvent) []TelnetEvent {
	owned := make([]TelnetEvent, len(events))
	copy(owned, events)
	for i := range owned {
		owned[i].Data = bytes.Clone(owned[i].Data)
	}
	return owned
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
			if msg.line {
				if !c.writeLine(cx, msg.data) {
					return
				}
				continue
			}

			data := msg.data
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

// writeLine writes one application line to the socket.
func (c *TCPClient) writeLine(cx *connection, data []byte) bool {
	// Line data is text: double IAC bytes so the server reads them as data,
	// not commands. Raw messages are protocol frames and bypass this path.
	if bytes.IndexByte(data, CmdIAC) >= 0 {
		data = EscapeIAC(data)
	}
	data = append(data, '\r', '\n')

	cx.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := cx.conn.Write(data)
	cx.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		cx.conn.Close()
		return false
	}
	return true
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
