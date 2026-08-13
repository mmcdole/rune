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
	"sync/atomic"
	"time"
)

// TCPClient owns the active TCP connection and exposes a stable output channel.
type TCPClient struct {
	// Stable across connections; blocking publication applies TCP backpressure.
	outputChan chan Output

	mu                  sync.Mutex
	current             *connection
	desiredConnectionID uint64 // Latest connection ID requested by Session

	// Last known window size, retained across connections so NAWS
	// can answer immediately on the next connect.
	width, height int
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
	output       *outputBuffer

	// textMu orders incoming text against send boundaries.
	textMu sync.Mutex

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

	gmcpActive atomic.Bool // GMCP negotiated on this connection

	localEcho atomic.Bool

	// writeLoop is the sole socket writer; other goroutines enqueue here.
	sendQueue chan outMsg

	// Signal to stop internal goroutines
	done      chan struct{}
	closeOnce sync.Once
}

// NewTCPClient creates a new client.
func NewTCPClient() *TCPClient {
	return &TCPClient{
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
		output:       &outputBuffer{},
		sendQueue:    make(chan outMsg, 4096),
		done:         make(chan struct{}),
	}
	cx.localEcho.Store(true)

	// Ignore a dial superseded by BeginConnect or Disconnect.
	c.mu.Lock()
	if c.desiredConnectionID != connectionID {
		c.mu.Unlock()
		conn.Close()
		return context.Canceled
	}
	// Snapshot the latest window size atomically with installation.
	cx.hs = newHandshake(useTLS, c.width, c.height)
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

// Send queues one game line. It fails without blocking if disconnected or
// the send queue is full.
func (c *TCPClient) Send(data string) error {
	if strings.ContainsAny(data, "\r\n") {
		return fmt.Errorf("game line cannot contain CR or LF")
	}
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

// GMCPActive reports whether GMCP is negotiated on the current
// connection. False when disconnected.
func (c *TCPClient) GMCPActive() bool {
	c.mu.Lock()
	cx := c.current
	c.mu.Unlock()
	return cx != nil && cx.gmcpActive.Load()
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

// publish tags output so events queued before reconnect can be ignored.
func (c *TCPClient) publish(cx *connection, out Output) bool {
	out.ConnectionID = cx.connectionID
	select {
	case c.outputChan <- out:
		return true
	case <-cx.done:
		return false
	}
}

// readLoop reads cx and blocks on output publication to apply TCP backpressure.
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
				c.publish(cx, Output{Kind: OutputDisconnect})
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
	events := cx.parser.Receive(data)

	// Queue replies before locking incoming text; a full send queue can block.
	for _, ev := range events {
		switch ev.Kind {
		case TelnetEventDataSend:
			if !cx.enqueueRaw(ev.Data) {
				return false
			}

		case TelnetEventNegotiation:
			for _, frame := range cx.hs.onNegotiation(ev.Command, ev.Option) {
				if !cx.enqueueRaw(frame) {
					return false
				}
			}

		case TelnetEventSubnegotiation:
			if ev.Option != OptMCCP2 && ev.Option != OptGMCP {
				for _, frame := range cx.hs.onSubnegotiation(ev.Option, ev.Data) {
					if !cx.enqueueRaw(frame) {
						return false
					}
				}
			}
		}
	}

	// Keep text from this read on one side of a send boundary.
	cx.textMu.Lock()
	locked := true
	defer func() {
		if locked {
			cx.textMu.Unlock()
		}
	}()

	// MCCP2 activation is deferred to the end of the batch: the parser
	// stops parsing at IAC SB 86 IAC SE and hands back the remaining
	// raw (compressed) bytes, so nothing after the marker is parsed.
	startMCCP := false
	var mccpRest []byte
	gmcpFinalActive := cx.gmcpActive.Load()
	deferGMCPMessages := false
	var deferredGMCP []Output
	sawText := false

	for _, ev := range events {
		switch ev.Kind {
		case TelnetEventDataReceive:
			sawText = true
			lines := cx.output.receive(ev.Data)
			for _, line := range lines {
				if !c.publish(cx, Output{Kind: OutputLine, Payload: string(line)}) {
					return false
				}
			}

		case TelnetEventIAC:
			if ev.Command == CmdGA || ev.Command == CmdEOR {
				text, completedLine := cx.output.consumePrompt()
				if completedLine {
					if !c.publish(cx, Output{Kind: OutputLine, Payload: text}) {
						return false
					}
				} else if text != "" {
					if !c.publish(cx, Output{Kind: OutputPrompt, Payload: text}) {
						return false
					}
				}
			}

		case TelnetEventNegotiation:
			cx.applyNegotiation(ev.Command, ev.Option)
			if ev.Option == OptGMCP {
				switch ev.Command {
				case CmdWILL, CmdDO:
					gmcpFinalActive = true
					if !cx.gmcpActive.Load() {
						deferGMCPMessages = true
					}
				case CmdWONT, CmdDONT:
					gmcpFinalActive = false
					deferGMCPMessages = false
					deferredGMCP = nil
					// Disable sends now; re-enable after the next reply is queued.
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
					out := Output{Kind: OutputGMCP, Package: pkg, Payload: payload}
					if deferGMCPMessages {
						deferredGMCP = append(deferredGMCP, out)
					} else {
						if !c.publish(cx, out) {
							return false
						}
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

	// Publish at most one partial line per read. GA/EOR may consume it first.
	if sawText {
		partial := cx.output.peekPartial()
		if partial != "" {
			if !c.publish(cx, Output{
				Kind:    OutputPartial,
				Payload: partial,
			}) {
				return false
			}
		}
	}

	cx.textMu.Unlock()
	locked = false
	if gmcpFinalActive && !cx.gmcpActive.Swap(true) {
		// The reply is queued first, so activation handlers may send immediately.
		if !c.publish(cx, Output{Kind: OutputGMCPEnabled}) {
			return false
		}
	}
	// Publish deferred GMCP messages after activation. A later disable in this
	// parse clears them.
	if gmcpFinalActive {
		for _, out := range deferredGMCP {
			if !c.publish(cx, out) {
				return false
			}
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

// enqueueRaw blocks until protocol data is queued or the connection closes.
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

// writeLine discards the current partial line and publishes a send boundary
// before writing.
func (c *TCPClient) writeLine(cx *connection, data []byte) bool {
	cx.textMu.Lock()

	cx.output.discardPartial()

	if !c.publish(cx, Output{Kind: OutputSendBoundary}) {
		cx.textMu.Unlock()
		return false
	}
	cx.textMu.Unlock()

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

// applyNegotiation updates local echo state from telnet negotiation.
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
	}
}
