package network

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

func TestSplitAddress(t *testing.T) {
	cases := []struct {
		in       string
		hostport string
		useTLS   bool
		insecure bool
		wantErr  bool
	}{
		{"mud.example.com:4000", "mud.example.com:4000", false, false, false},
		{"telnet://mud.example.com:4000", "mud.example.com:4000", false, false, false},
		{"tcp://mud.example.com:4000", "mud.example.com:4000", false, false, false},
		{"tls://mud.example.com:4000", "mud.example.com:4000", true, false, false},
		{"tls+insecure://mud.example.com:4000", "mud.example.com:4000", true, true, false},
		{"gopher://mud.example.com:4000", "", false, false, true},
	}
	for _, c := range cases {
		hostport, useTLS, insecure, err := splitAddress(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("splitAddress(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("splitAddress(%q): %v", c.in, err)
			continue
		}
		if hostport != c.hostport || useTLS != c.useTLS || insecure != c.insecure {
			t.Errorf("splitAddress(%q) = (%q, %v, %v), want (%q, %v, %v)",
				c.in, hostport, useTLS, insecure, c.hostport, c.useTLS, c.insecure)
		}
	}
}

func TestConnectRejectsSupersededRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	const staleID, currentID = 1, 2
	c := NewTCPClient()
	c.BeginConnect(currentID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = c.Connect(ctx, ln.Addr().String(), staleID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stale Connect error = %v, want context.Canceled", err)
	}
	if c.current != nil {
		t.Fatal("stale Connect installed a connection")
	}
}

// selfSignedServer starts a TLS listener with a throwaway self-signed
// certificate that writes greeting to every connection.
func selfSignedServer(t *testing.T, greeting string) net.Listener {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "rune-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				c.Write([]byte(greeting)) // handshake failures surface here; ignored
				c.Close()
			}(conn)
		}
	}()
	return ln
}

func TestTLSConnectInsecure(t *testing.T) {
	ln := selfSignedServer(t, "hello\r\n")

	c := NewTCPClient()
	defer c.Disconnect()
	c.BeginConnect(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, "tls+insecure://"+ln.Addr().String(), 1); err != nil {
		t.Fatalf("connect: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			if out.Kind == OutputLine {
				if out.Payload != "hello" {
					t.Fatalf("got line %q, want %q", out.Payload, "hello")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for greeting over TLS")
		}
	}
}

func TestTLSVerificationRejectsSelfSigned(t *testing.T) {
	ln := selfSignedServer(t, "hello\r\n")

	c := NewTCPClient()
	c.BeginConnect(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, "tls://"+ln.Addr().String(), 1); err == nil {
		c.Disconnect()
		t.Fatal("expected certificate verification failure for tls:// against self-signed cert")
	}
}

// --- Protocol loopback helpers ---

// telnetServer starts a plain TCP listener that runs script on the
// first accepted connection. Returns the address to dial.
func telnetServer(t *testing.T, script func(t *testing.T, conn net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		script(t, conn)
	}()
	return ln.Addr().String()
}

// connectLoopback dials the test server and cleans up the client.
func connectLoopback(t *testing.T, addr string) *TCPClient {
	t.Helper()
	c := NewTCPClient()
	t.Cleanup(c.Disconnect)
	c.BeginConnect(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, addr, 1); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return c
}

// expectBytes reads from conn until want appears in the stream (or
// times out). Returns everything read.
func expectBytes(t *testing.T, conn net.Conn, want []byte, what string) []byte {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	var got []byte
	buf := make([]byte, 512)
	for !bytes.Contains(got, want) {
		n, err := conn.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err != nil {
			t.Fatalf("%s: wanted %v in stream, got %v (read error: %v)", what, want, got, err)
		}
	}
	return got
}

// nextOutput waits for the next output of the given kind.
func nextOutput(t *testing.T, c *TCPClient, kind OutputKind, what string) Output {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			if out.Kind == kind {
				return out
			}
			if out.Kind == OutputDisconnect && kind != OutputDisconnect {
				t.Fatalf("%s: connection dropped while waiting", what)
			}
		case <-deadline:
			t.Fatalf("%s: timed out waiting for output kind %d", what, kind)
		}
	}
}

// blockedWriteConn makes a socket write observable and keeps it blocked until
// release is called. It lets ordering tests exercise the text lock without
// relying on kernel buffer sizes.
type blockedWriteConn struct {
	writeStarted chan struct{}
	writeRelease chan struct{}
	startOnce    sync.Once
	releaseOnce  sync.Once
}

func newBlockedWriteConn() *blockedWriteConn {
	return &blockedWriteConn{
		writeStarted: make(chan struct{}),
		writeRelease: make(chan struct{}),
	}
}

func (c *blockedWriteConn) Read([]byte) (int, error) { return 0, net.ErrClosed }

func (c *blockedWriteConn) Write(p []byte) (int, error) {
	c.startOnce.Do(func() { close(c.writeStarted) })
	<-c.writeRelease
	return len(p), nil
}

func (c *blockedWriteConn) Close() error {
	c.release()
	return nil
}

func (c *blockedWriteConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *blockedWriteConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *blockedWriteConn) SetDeadline(time.Time) error      { return nil }
func (c *blockedWriteConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockedWriteConn) SetWriteDeadline(time.Time) error { return nil }

func (c *blockedWriteConn) release() {
	c.releaseOnce.Do(func() { close(c.writeRelease) })
}

// --- Identity negotiation over a real socket ---

func TestIdentityNegotiationLoopback(t *testing.T) {
	done := make(chan struct{})
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		defer close(done)

		// TTYPE: DO -> WILL, SEND -> IS RUNE
		conn.Write([]byte{CmdIAC, CmdDO, OptTTYPE})
		expectBytes(t, conn, []byte{CmdIAC, CmdWILL, OptTTYPE}, "WILL TTYPE")
		conn.Write(subnegFrame(OptTTYPE, []byte{CmdSEND}))
		wantTTYPE := subnegFrame(OptTTYPE, append([]byte{CmdIS}, []byte("RUNE")...))
		expectBytes(t, conn, wantTTYPE, "TTYPE IS RUNE")

		// NAWS: DO -> WILL + size report (set before connect)
		conn.Write([]byte{CmdIAC, CmdDO, OptNAWS})
		expectBytes(t, conn, subnegFrame(OptNAWS, []byte{0, 100, 0, 30}), "NAWS 100x30")

		// CHARSET: server WILL, then REQUEST -> ACCEPTED UTF-8
		conn.Write([]byte{CmdIAC, CmdWILL, OptCharset})
		expectBytes(t, conn, []byte{CmdIAC, CmdDO, OptCharset}, "DO CHARSET")
		conn.Write(subnegFrame(OptCharset, append([]byte{charsetRequest}, []byte(";UTF-8")...)))
		wantCharset := subnegFrame(OptCharset, append([]byte{charsetAccepted}, []byte("UTF-8")...))
		expectBytes(t, conn, wantCharset, "CHARSET ACCEPTED UTF-8")

		// NEW-ENVIRON: DO -> WILL, SEND CLIENT_NAME -> IS ... RUNE
		conn.Write([]byte{CmdIAC, CmdDO, OptNewEnviron})
		expectBytes(t, conn, []byte{CmdIAC, CmdWILL, OptNewEnviron}, "WILL NEW-ENVIRON")
		req := append([]byte{environSEND, environVAR}, []byte("CLIENT_NAME")...)
		conn.Write(subnegFrame(OptNewEnviron, req))
		var reply []byte
		reply = append(reply, environIS, environVAR)
		reply = append(reply, []byte("CLIENT_NAME")...)
		reply = append(reply, environVALUE)
		reply = append(reply, []byte("RUNE")...)
		expectBytes(t, conn, subnegFrame(OptNewEnviron, reply), "NEW-ENVIRON IS CLIENT_NAME")
	})

	c := NewTCPClient()
	t.Cleanup(c.Disconnect)
	c.SetWindowSize(100, 30) // retained for the upcoming connection
	c.BeginConnect(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, addr, 1); err != nil {
		t.Fatalf("connect: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("server script did not complete")
	}
}

// --- MCCP2 ---

// TestMCCP2DecompressAndResume verifies the full MCCP2 lifecycle:
// negotiation, decompression of the zlib stream, and a clean stream
// end resuming plain telnet without losing the bytes that follow.
func TestMCCP2DecompressAndResume(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte{CmdIAC, CmdWILL, OptMCCP2})
		expectBytes(t, conn, []byte{CmdIAC, CmdDO, OptMCCP2}, "DO MCCP2")

		// Compression marker + complete zlib stream + plain trailer,
		// all in one burst - exercises the byte-exact handoff.
		var payload bytes.Buffer
		payload.Write([]byte{CmdIAC, CmdSB, OptMCCP2, CmdIAC, CmdSE})
		zw := zlib.NewWriter(&payload)
		zw.Write([]byte("compressed one\r\ncompressed two\r\n"))
		zw.Close() // Z_STREAM_END: compression over
		payload.Write([]byte("plain after stream\r\n"))
		conn.Write(payload.Bytes())

		// Keep the connection open until the test finishes reading
		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)

	for _, want := range []string{"compressed one", "compressed two", "plain after stream"} {
		out := nextOutput(t, c, OutputLine, "line "+want)
		if out.Payload != want {
			t.Fatalf("got line %q, want %q", out.Payload, want)
		}
	}
}

// TestMCCP2SplitAcrossReads verifies compression works when the
// compressed bytes arrive in a separate TCP segment from the
// activation marker.
func TestMCCP2SplitAcrossReads(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte{CmdIAC, CmdWILL, OptMCCP2})
		expectBytes(t, conn, []byte{CmdIAC, CmdDO, OptMCCP2}, "DO MCCP2")

		// Marker alone in one segment...
		conn.Write([]byte{CmdIAC, CmdSB, OptMCCP2, CmdIAC, CmdSE})
		time.Sleep(50 * time.Millisecond)

		// ...compressed data in the next
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write([]byte("split activation\r\n"))
		zw.Close()
		conn.Write(z.Bytes())

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	out := nextOutput(t, c, OutputLine, "split-activation line")
	if out.Payload != "split activation" {
		t.Fatalf("got line %q, want %q", out.Payload, "split activation")
	}
}

// TestMCCP2CorruptStreamDisconnects verifies a broken zlib stream is
// treated as a hard connection error (the stream is unrecoverable),
// not a hang or a crash.
func TestMCCP2CorruptStreamDisconnects(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte{CmdIAC, CmdWILL, OptMCCP2})
		expectBytes(t, conn, []byte{CmdIAC, CmdDO, OptMCCP2}, "DO MCCP2")
		conn.Write([]byte{CmdIAC, CmdSB, OptMCCP2, CmdIAC, CmdSE})
		conn.Write([]byte("this is not zlib data"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	nextOutput(t, c, OutputDisconnect, "disconnect on corrupt stream")
}

func TestProtocolRepliesQueueBeforeTextLock(t *testing.T) {
	c := &TCPClient{outputChan: make(chan Output, 8)}
	cx := &connection{
		parser:    NewParser(defaultCompatibility()),
		output:    &outputBuffer{},
		hs:        newHandshake(false, 0, 0),
		sendQueue: make(chan outMsg, 1),
		done:      make(chan struct{}),
	}
	t.Cleanup(func() { close(cx.done) })

	// If reply queuing took textMu, neither reply could pass this one-slot queue.
	cx.textMu.Lock()
	textLocked := true
	defer func() {
		if textLocked {
			cx.textMu.Unlock()
		}
	}()
	cx.sendQueue <- outMsg{data: []byte("look"), line: true}

	wire := []byte{CmdIAC, CmdWILL, OptEOR, CmdIAC, CmdWILL, OptSGA}
	wire = append(wire, []byte("marker\r\n")...)
	processed := make(chan bool, 1)
	go func() { processed <- c.processIncoming(cx, wire) }()

	nextQueued := func(what string) outMsg {
		t.Helper()
		select {
		case msg := <-cx.sendQueue:
			return msg
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
			return outMsg{}
		}
	}

	if got := string(nextQueued("queued game line").data); got != "look" {
		t.Fatalf("first queued message = %q, want look", got)
	}
	if got := nextQueued("first protocol reply").data; !bytes.Equal(got, []byte{CmdIAC, CmdDO, OptEOR}) {
		t.Fatalf("first reply = %v, want DO EOR", got)
	}
	if got := nextQueued("second protocol reply").data; !bytes.Equal(got, []byte{CmdIAC, CmdDO, OptSGA}) {
		t.Fatalf("second reply = %v, want DO SGA", got)
	}
	cx.textMu.Unlock()
	textLocked = false

	select {
	case ok := <-processed:
		if !ok {
			t.Fatal("processIncoming stopped after queuing replies")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("processIncoming did not resume after textMu was released")
	}
	if got := nextOutput(t, c, OutputLine, "batch text after replies").Payload; got != "marker" {
		t.Fatalf("batch line = %q, want marker", got)
	}
}

// --- GMCP ---

func TestGMCPReplyPrecedesActivationHandlerSend(t *testing.T) {
	c := &TCPClient{outputChan: make(chan Output)}
	cx := &connection{
		parser:    NewParser(defaultCompatibility()),
		output:    &outputBuffer{},
		hs:        newHandshake(false, 0, 0),
		sendQueue: make(chan outMsg, 2),
		done:      make(chan struct{}),
	}
	c.current = cx

	wire := append([]byte{CmdIAC, CmdWILL, OptGMCP},
		subnegFrame(OptGMCP, []byte(`Char.Vitals {"hp":100}`))...)
	processed := make(chan bool, 1)
	go func() { processed <- c.processIncoming(cx, wire) }()

	nextOutput(t, c, OutputGMCPEnabled, "GMCP activation")
	if err := c.SendGMCP("Core.Hello", `{"client":"Rune"}`); err != nil {
		t.Fatalf("activation handler send: %v", err)
	}
	message := nextOutput(t, c, OutputGMCP, "same-parse GMCP message")
	if message.Package != "Char.Vitals" || message.Payload != `{"hp":100}` {
		t.Fatalf("GMCP message = (%q, %q)", message.Package, message.Payload)
	}
	select {
	case ok := <-processed:
		if !ok {
			t.Fatal("processIncoming stopped during GMCP activation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("processIncoming did not finish after GMCP activation")
	}

	reply := <-cx.sendQueue
	if want := []byte{CmdIAC, CmdDO, OptGMCP}; reply.line || !bytes.Equal(reply.data, want) {
		t.Fatalf("negotiation reply = %v, want %v", reply.data, want)
	}
	sent := <-cx.sendQueue
	if want := subnegFrame(OptGMCP, []byte(`Core.Hello {"client":"Rune"}`)); sent.line || !bytes.Equal(sent.data, want) {
		t.Fatalf("handler send = %+v, want raw GMCP after reply", sent)
	}
}

func TestGMCPBatchUsesFinalEnablement(t *testing.T) {
	tests := []struct {
		name       string
		wire       []byte
		wantActive bool
		want       []Output
	}{
		{
			name: "disabled",
			wire: append(append([]byte{CmdIAC, CmdWILL, OptGMCP},
				subnegFrame(OptGMCP, []byte(`Old.Payload {"old":true}`))...),
				CmdIAC, CmdWONT, OptGMCP),
		},
		{
			name: "re-enabled",
			wire: append(append(append([]byte{CmdIAC, CmdWILL, OptGMCP},
				subnegFrame(OptGMCP, []byte(`Old.Payload {"old":true}`))...),
				CmdIAC, CmdWONT, OptGMCP, CmdIAC, CmdWILL, OptGMCP),
				subnegFrame(OptGMCP, []byte(`New.Payload {"new":true}`))...),
			wantActive: true,
			want: []Output{
				{Kind: OutputGMCPEnabled},
				{Kind: OutputGMCP, Package: "New.Payload", Payload: `{"new":true}`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TCPClient{outputChan: make(chan Output, 4)}
			cx := &connection{
				parser:    NewParser(defaultCompatibility()),
				output:    &outputBuffer{},
				hs:        newHandshake(false, 0, 0),
				sendQueue: make(chan outMsg, 4),
				done:      make(chan struct{}),
			}

			if !c.processIncoming(cx, tt.wire) {
				t.Fatal("processIncoming stopped")
			}
			if got := cx.gmcpActive.Load(); got != tt.wantActive {
				t.Fatalf("GMCP active = %v, want %v", got, tt.wantActive)
			}

			var got []Output
			for len(c.outputChan) > 0 {
				got = append(got, <-c.outputChan)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("outputs = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("output %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGMCPLoopback(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte{CmdIAC, CmdWILL, OptGMCP})
		expectBytes(t, conn, []byte{CmdIAC, CmdDO, OptGMCP}, "DO GMCP")

		conn.Write(subnegFrame(OptGMCP, []byte(`Char.Vitals {"hp":100,"maxhp":200}`)))

		// Then wait for the client's own GMCP message
		want := subnegFrame(OptGMCP, []byte(`Core.Hello {"client":"Rune"}`))
		expectBytes(t, conn, want, "client Core.Hello frame")
		conn.Write([]byte("GMCP-send-marker\r\n"))
	})

	c := connectLoopback(t, addr)

	nextOutput(t, c, OutputGMCPEnabled, "GMCP enabled notification")

	out := nextOutput(t, c, OutputGMCP, "GMCP message")
	if out.Package != "Char.Vitals" || out.Payload != `{"hp":100,"maxhp":200}` {
		t.Fatalf("GMCP message = (%q, %q), want (Char.Vitals, json)", out.Package, out.Payload)
	}

	if err := c.SendGMCP("Core.Hello", `{"client":"Rune"}`); err != nil {
		t.Fatalf("SendGMCP: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputSendBoundary:
				t.Fatal("GMCP send published a send boundary")
			case OutputLine:
				if out.Payload != "GMCP-send-marker" {
					t.Fatalf("line after GMCP send = %q", out.Payload)
				}
				return
			case OutputDisconnect:
				t.Fatal("connection dropped before the GMCP send marker")
			}
		case <-deadline:
			t.Fatal("server never acknowledged the client's GMCP frame")
		}
	}
}

// TestGMCPSendRequiresNegotiation verifies sends fail cleanly before
// the server has negotiated GMCP.
func TestGMCPSendRequiresNegotiation(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	if err := c.SendGMCP("Core.Hello", "{}"); err == nil {
		t.Fatal("expected error sending GMCP before negotiation")
	}
}

// --- Game lines and prompts ---

// TestSendEscapesIAC verifies outgoing line data doubles IAC bytes so
// the server reads them as data. Protocol frames stay untouched - the
// negotiation loopback tests pin those byte-exact.
func TestSendEscapesIAC(t *testing.T) {
	done := make(chan struct{})
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		defer close(done)
		expectBytes(t, conn, []byte{'a', 0xFF, 0xFF, 'b', '\r', '\n'}, "escaped line send")
	})

	c := connectLoopback(t, addr)
	if err := c.Send("a\xffb"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server never saw the escaped bytes")
	}
}

func TestSendRejectsLineSeparators(t *testing.T) {
	c := NewTCPClient()
	c.current = &connection{sendQueue: make(chan outMsg, 2), done: make(chan struct{})}
	for _, data := range []string{"north\nlook", "north\rlook"} {
		if err := c.Send(data); err == nil {
			t.Fatalf("Send(%q) succeeded", data)
		}
	}
	if got := len(c.current.sendQueue); got != 0 {
		t.Fatalf("rejected lines queued %d writes", got)
	}
}

func TestPromptMarksConsumePartialLineOnce(t *testing.T) {
	for _, tt := range []struct {
		name string
		wire []byte
		want []Output
	}{
		{
			name: "GA",
			wire: append([]byte("the sun rises\r\nHP:100> "), CmdIAC, CmdGA),
			want: []Output{
				{Kind: OutputLine, Payload: "the sun rises"},
				{Kind: OutputPrompt, Payload: "HP:100> "},
			},
		},
		{
			name: "EOR",
			wire: append([]byte("the sun rises\r\nHP:100> "), CmdIAC, CmdEOR),
			want: []Output{
				{Kind: OutputLine, Payload: "the sun rises"},
				{Kind: OutputPrompt, Payload: "HP:100> "},
			},
		},
		{name: "empty GA", wire: []byte{CmdIAC, CmdGA}},
		{name: "empty EOR", wire: []byte{CmdIAC, CmdEOR}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &TCPClient{outputChan: make(chan Output, 3)}
			cx := &connection{
				parser:    NewParser(defaultCompatibility()),
				output:    &outputBuffer{},
				sendQueue: make(chan outMsg, 1),
				done:      make(chan struct{}),
			}

			if !c.processIncoming(cx, tt.wire) {
				t.Fatal("processIncoming stopped")
			}
			var got []Output
			for len(c.outputChan) > 0 {
				got = append(got, <-c.outputChan)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("outputs = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("output %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPartialLineGrowsAcrossReadsAfterGA(t *testing.T) {
	advance := make(chan struct{})
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write(append([]byte("HP:100> "), CmdIAC, CmdGA))
		<-advance
		conn.Write([]byte("User"))
		<-advance
		conn.Write([]byte("name:"))
		<-advance
		conn.Write([]byte(" accepted\r\n"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	if got := nextOutput(t, c, OutputPrompt, "GA prompt").Payload; got != "HP:100> " {
		t.Fatalf("prompt = %q", got)
	}
	advance <- struct{}{}
	if got := nextOutput(t, c, OutputPartial, "first partial").Payload; got != "User" {
		t.Fatalf("first partial = %q, want User", got)
	}
	advance <- struct{}{}
	if got := nextOutput(t, c, OutputPartial, "grown partial").Payload; got != "Username:" {
		t.Fatalf("grown partial = %q, want Username:", got)
	}
	advance <- struct{}{}
	if got := nextOutput(t, c, OutputLine, "complete line").Payload; got != "Username: accepted" {
		t.Fatalf("complete line = %q, want %q", got, "Username: accepted")
	}
}

func TestSentLinesPublishBoundariesBeforeLaterServerText(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte("first half"))
		expectBytes(t, conn, []byte("look\r\nscore\r\n"), "game lines after partial")
		conn.Write([]byte(" second half\r\nnext tail"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	first := nextOutput(t, c, OutputPartial, "first partial")
	if first.Payload != "first half" {
		t.Fatalf("first partial = %q", first.Payload)
	}
	if err := c.Send("look"); err != nil {
		t.Fatal(err)
	}
	if err := c.Send("score"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	boundaries := 0
	sawTruncatedLine := false
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputSendBoundary:
				boundaries++
			case OutputLine:
				if boundaries != 2 {
					t.Fatalf("response line followed %d boundaries, want 2", boundaries)
				}
				if out.Payload != " second half" {
					t.Fatalf("line after mid-fragment send = %q", out.Payload)
				}
				sawTruncatedLine = true
			case OutputPartial:
				if !sawTruncatedLine {
					t.Fatalf("new partial arrived before response line: %q", out.Payload)
				}
				if boundaries != 2 {
					t.Fatalf("new partial followed %d boundaries, want 2", boundaries)
				}
				if out.Payload != "next tail" {
					t.Fatalf("new partial = %q", out.Payload)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for boundary and post-command output")
		}
	}
}

func TestSentLineEmitsBoundaryAfterGAPrompt(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write(append([]byte("HP:100> "), CmdIAC, CmdGA))
		expectBytes(t, conn, []byte("look\r\n"), "command after confirmed prompt")
		conn.Write([]byte("marker\r\n"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	nextOutput(t, c, OutputPrompt, "confirmed prompt")
	if err := c.Send("look"); err != nil {
		t.Fatal(err)
	}
	nextOutput(t, c, OutputSendBoundary, "send boundary")
	if got := nextOutput(t, c, OutputLine, "response line").Payload; got != "marker" {
		t.Fatalf("response line = %q", got)
	}
}

func TestBlockedLineWriteDoesNotStopIncomingParsing(t *testing.T) {
	conn := newBlockedWriteConn()
	t.Cleanup(conn.release)
	c := &TCPClient{outputChan: make(chan Output, 4)}
	cx := &connection{
		conn:      conn,
		parser:    NewParser(defaultCompatibility()),
		output:    &outputBuffer{},
		sendQueue: make(chan outMsg, 1),
		done:      make(chan struct{}),
	}

	writeDone := make(chan bool, 1)
	go func() { writeDone <- c.writeLine(cx, []byte("look")) }()
	nextOutput(t, c, OutputSendBoundary, "boundary before blocked write")
	<-conn.writeStarted

	parseStarted := make(chan struct{})
	parseDone := make(chan bool, 1)
	go func() {
		close(parseStarted)
		parseDone <- c.processIncoming(cx, []byte("write-block-marker\r\n"))
	}()
	<-parseStarted

	select {
	case out := <-c.outputChan:
		if out.Kind != OutputLine || out.Payload != "write-block-marker" {
			t.Fatalf("incoming output during blocked write = %+v", out)
		}
	case <-time.After(time.Second):
		t.Fatal("incoming parsing blocked behind the socket write")
	}

	conn.release()
	if ok := <-writeDone; !ok {
		t.Fatal("writeLine failed after releasing the socket")
	}
	if ok := <-parseDone; !ok {
		t.Fatal("processIncoming stopped during blocked write test")
	}
}
