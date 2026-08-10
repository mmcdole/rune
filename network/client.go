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

// outMsg is a queued write. Line messages are commands: they end the current
// partial epoch, discard its accumulator, and gain CRLF. Raw messages are
// protocol bytes such as Telnet negotiation replies, written verbatim.
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

	// textMu orders complete incoming read batches against outbound command
	// boundaries. A boundary is published before its command reaches the wire,
	// and no response batch can be published ahead of it.
	textMu       sync.Mutex
	partialEpoch uint64

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
	conn, err := d.DialContext(ctx, "tcp", dialOverride(hostport))
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
		conn:         conn,
		reader:       conn,
		raw:          conn,
		hs:           newHandshake(useTLS, c.width, c.height),
		parser:       NewParser(defaultCompatibility()),
		output:       NewOutputBuffer(),
		sendQueue:    make(chan outMsg, 4096),
		done:         make(chan struct{}),
		partialEpoch: 1,
	}
	cx.localEcho.Store(true)

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
	// A socket read is one indivisible observation of the incoming stream.
	// Serialize the whole batch with command boundaries so a boundary cannot
	// discard half a batch or let post-command output overtake its control
	// event. MCCP setup below is deliberately outside this lock because
	// zlib.NewReader may need to read more bytes from the socket.
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
	var rawFrames [][]byte
	sawText := false

	for _, ev := range cx.parser.Receive(data) {
		switch ev.Kind {
		case TelnetEventDataSend:
			// Queue replies after releasing textMu. enqueueRaw is lossless and
			// may block behind user commands; blocking here could deadlock with
			// writeLoop waiting for this batch's text boundary lock.
			rawFrames = append(rawFrames, ev.Data)

		case TelnetEventDataReceive:
			sawText = true
			lines := cx.output.Receive(ev.Data)
			for _, line := range lines {
				select {
				case c.outputChan <- Output{Kind: OutputLine, Payload: string(line)}:
				case <-cx.done:
					return false
				}
			}

		case TelnetEventIAC:
			if ev.Command == CmdGA || ev.Command == CmdEOR {
				text, completedLine := cx.output.ConsumePrompt()
				if completedLine {
					select {
					case c.outputChan <- Output{Kind: OutputLine, Payload: text}:
					case <-cx.done:
						return false
					}
				} else if text != "" {
					terminator := PromptTerminatorGA
					if ev.Command == CmdEOR {
						terminator = PromptTerminatorEOR
					}
					select {
					case c.outputChan <- Output{
						Kind:             OutputPrompt,
						Payload:          text,
						PromptTerminator: terminator,
						PartialEpoch:     cx.partialEpoch,
					}:
					case <-cx.done:
						return false
					}
				}
			}

		case TelnetEventNegotiation:
			cx.applyNegotiation(ev.Command, ev.Option)
			for _, frame := range cx.hs.onNegotiation(ev.Command, ev.Option) {
				rawFrames = append(rawFrames, frame)
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
					rawFrames = append(rawFrames, frame)
				}
			}

		case TelnetEventDecompressImmediate:
			// Raw compressed bytes that followed IAC SB 86 IAC SE in
			// the same read. Always the final event of a batch.
			startMCCP = true
			mccpRest = ev.Data
		}
	}

	// An unfinished-tail peek runs once per batch, AFTER any GA/EOR in
	// the same batch has consumed the buffer. Peeking inside the
	// DataReceive case emitted "HP:100> " twice for a single
	// "HP:100> " + IAC GA read - once from the peek, once from the GA
	// flush - and the session committed the duplicate to scrollback.
	if sawText {
		partial := cx.output.PeekPartial()
		if partial != "" {
			select {
			case c.outputChan <- Output{
				Kind:         OutputPartial,
				Payload:      partial,
				PartialEpoch: cx.partialEpoch,
			}:
			case <-cx.done:
				return false
			}
		}
	}

	cx.textMu.Unlock()
	locked = false
	for _, frame := range rawFrames {
		if !cx.enqueueRaw(frame) {
			return false
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

// writeLine publishes and applies one local text boundary atomically with the
// socket write. The boundary always fires, even when GA/EOR already consumed
// the accumulator: a prompt trigger may enqueue a send before Session has
// finished painting that confirmed prompt. Holding textMu through Write keeps
// the server's response behind the boundary on outputChan.
func (c *TCPClient) writeLine(cx *connection, data []byte) bool {
	cx.textMu.Lock()
	defer cx.textMu.Unlock()

	endedEpoch := cx.partialEpoch
	cx.partialEpoch++
	cx.output.DiscardPartial()

	select {
	case c.outputChan <- Output{
		Kind:         OutputSendBoundary,
		PartialEpoch: endedEpoch,
	}:
	case <-cx.done:
		return false
	}

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
