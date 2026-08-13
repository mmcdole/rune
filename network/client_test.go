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
	driven := driveProtocol(t, c, 1, 0, 0)

	if got := dataThrough(t, driven, "hello\r\n", "TLS greeting"); got != "hello\r\n" {
		t.Fatalf("got data %q, want %q", got, "hello\r\n")
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

// protocolClient drives Protocol synchronously over a real TCPClient and keeps
// application effects observable to a test.
type protocolClient struct {
	*TCPClient
	connectionID uint64
	protocol     *Protocol
	effects      chan Effect
	disconnected chan struct{}
}

func driveProtocol(t *testing.T, client *TCPClient, connectionID uint64, width, height int) *protocolClient {
	t.Helper()
	driven := &protocolClient{
		TCPClient:    client,
		connectionID: connectionID,
		protocol:     NewProtocol(width, height),
		effects:      make(chan Effect, 256),
		disconnected: make(chan struct{}),
	}
	go func() {
		for inbound := range client.Inbound() {
			if inbound.ConnectionID != connectionID {
				continue
			}
			if inbound.Kind == InboundDisconnect {
				close(driven.disconnected)
				return
			}
			ok := driven.protocol.Process(inbound.Batch, func(effect Effect) bool {
				if effect.Kind == EffectSendFrame {
					if err := client.SendFrame(connectionID, effect.Data); err != nil {
						return false
					}
				}
				driven.effects <- effect
				return true
			})
			if !ok {
				return
			}
		}
	}()
	return driven
}

func connectLoopbackSize(t *testing.T, addr string, width, height int) *protocolClient {
	t.Helper()
	c := NewTCPClient()
	t.Cleanup(c.Disconnect)
	c.BeginConnect(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, addr, 1); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return driveProtocol(t, c, 1, width, height)
}

func connectLoopback(t *testing.T, addr string) *protocolClient {
	t.Helper()
	return connectLoopbackSize(t, addr, 0, 0)
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

// nextEffect waits for the next protocol effect of the given kind.
func nextEffect(t *testing.T, c *protocolClient, kind EffectKind, what string) Effect {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case effect := <-c.effects:
			if effect.Kind == kind {
				return effect
			}
		case <-c.disconnected:
			t.Fatalf("%s: connection dropped while waiting", what)
		case <-deadline:
			t.Fatalf("%s: timed out waiting for effect kind %d", what, kind)
		}
	}
}

// dataThrough collects application-data events until want appears. Network
// deliberately exposes Telnet data rather than socket-read boundaries, so
// loopback tests compare the reconstructed stream instead of individual reads.
func dataThrough(t *testing.T, c *protocolClient, want, what string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var got bytes.Buffer
	for {
		select {
		case effect := <-c.effects:
			switch effect.Kind {
			case EffectServerData:
				got.Write(effect.Data)
				if bytes.Contains(got.Bytes(), []byte(want)) {
					return got.String()
				}
			}
		case <-c.disconnected:
			t.Fatalf("%s: connection dropped after data %q", what, got.String())
		case <-deadline:
			t.Fatalf("%s: timed out waiting for %q in data %q", what, want, got.String())
		}
	}
}

// blockedWriteConn makes a socket write observable and keeps it blocked until
// release is called. It lets concurrency tests avoid relying on kernel buffer
// sizes.
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

	connectLoopbackSize(t, addr, 100, 30)

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

	want := "compressed one\r\ncompressed two\r\nplain after stream\r\n"
	if got := dataThrough(t, c, "plain after stream\r\n", "MCCP stream and trailer"); got != want {
		t.Fatalf("got data %q, want %q", got, want)
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
	if got := dataThrough(t, c, "split activation\r\n", "split-activation data"); got != "split activation\r\n" {
		t.Fatalf("got data %q, want %q", got, "split activation\r\n")
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
	select {
	case <-c.disconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for disconnect on corrupt stream")
	}
}

// --- GMCP ---

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

	nextEffect(t, c, EffectGMCPEnabled, "GMCP enabled notification")

	out := nextEffect(t, c, EffectGMCPMessage, "GMCP message")
	if out.Package != "Char.Vitals" || out.Payload != `{"hp":100,"maxhp":200}` {
		t.Fatalf("GMCP message = (%q, %q), want (Char.Vitals, json)", out.Package, out.Payload)
	}

	frame, err := c.protocol.GMCPFrame("Core.Hello", `{"client":"Rune"}`)
	if err != nil {
		t.Fatalf("GMCPFrame: %v", err)
	}
	if err := c.SendFrame(c.connectionID, frame); err != nil {
		t.Fatalf("SendFrame: %v", err)
	}
	if got := dataThrough(t, c, "GMCP-send-marker\r\n", "GMCP send marker"); got != "GMCP-send-marker\r\n" {
		t.Fatalf("data after GMCP send = %q", got)
	}
}

// --- Game lines and Telnet record marks ---

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
	if err := c.SendLine(c.connectionID, "a\xffb"); err != nil {
		t.Fatalf("SendLine: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server never saw the escaped bytes")
	}
}

func TestSendRejectsLineSeparators(t *testing.T) {
	c := NewTCPClient()
	c.current = &connection{connectionID: 9, sendQueue: make(chan outMsg, 2), done: make(chan struct{})}
	for _, data := range []string{"north\nlook", "north\rlook"} {
		if err := c.SendLine(9, data); err == nil {
			t.Fatalf("SendLine(%q) succeeded", data)
		}
	}
	if got := len(c.current.sendQueue); got != 0 {
		t.Fatalf("rejected lines queued %d writes", got)
	}
}

func TestSendsStayOnTheIdentifiedConnectionAndOwnFrameBytes(t *testing.T) {
	c := NewTCPClient()
	c.current = &connection{connectionID: 2, sendQueue: make(chan outMsg, 2), done: make(chan struct{})}

	if err := c.SendLine(1, "look"); err == nil {
		t.Fatal("stale connection ID sent a game line")
	}
	frame := []byte{CmdIAC, CmdDO, OptEOR}
	if err := c.SendFrame(2, frame); err != nil {
		t.Fatalf("SendFrame: %v", err)
	}
	frame[2] = OptEcho

	queued := <-c.current.sendQueue
	if want := []byte{CmdIAC, CmdDO, OptEOR}; !bytes.Equal(queued.data, want) {
		t.Fatalf("queued frame = %v, want owned bytes %v", queued.data, want)
	}
}

func TestPromptMarksStayInTheirParserBatch(t *testing.T) {
	for _, tt := range []struct {
		name    string
		command byte
	}{
		{name: "GA", command: CmdGA},
		{name: "EOR", command: CmdEOR},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &TCPClient{inboundChan: make(chan Inbound, 1)}
			cx := &connection{
				connectionID: 7,
				parser:       NewParser(defaultCompatibility()),
				done:         make(chan struct{}),
			}

			wire := append([]byte("the sun rises\r\nHP:100> "), CmdIAC, tt.command)
			if !c.processIncoming(cx, wire) {
				t.Fatal("processIncoming stopped")
			}
			got := <-c.Inbound()
			if got.Kind != InboundEvents || got.ConnectionID != 7 {
				t.Fatalf("inbound envelope = %+v", got)
			}
			if len(got.Batch.Events) != 2 {
				t.Fatalf("events = %+v, want data and mark in one batch", got.Batch.Events)
			}
			if event := got.Batch.Events[0]; event.Kind != TelnetEventDataReceive || string(event.Data) != "the sun rises\r\nHP:100> " {
				t.Fatalf("first event = %+v, want prompt data", event)
			}
			if event := got.Batch.Events[1]; event.Kind != TelnetEventIAC || event.Command != tt.command {
				t.Fatalf("second event = %+v, want mark %d", event, tt.command)
			}
		})
	}
}

func TestCloneEventsOwnsEventStructsAndData(t *testing.T) {
	sourceData := []byte("payload")
	source := []TelnetEvent{{
		Kind:    TelnetEventSubnegotiation,
		Command: CmdSB,
		Option:  OptGMCP,
		Data:    sourceData,
	}}

	cloned := cloneEvents(source)
	source[0].Kind = TelnetEventIAC
	source[0].Command = CmdGA
	source[0].Option = OptEcho
	sourceData[0] = 'X'

	want := TelnetEvent{
		Kind:    TelnetEventSubnegotiation,
		Command: CmdSB,
		Option:  OptGMCP,
		Data:    []byte("payload"),
	}
	got := cloned[0]
	if got.Kind != want.Kind || got.Command != want.Command || got.Option != want.Option || !bytes.Equal(got.Data, want.Data) {
		t.Fatalf("cloned event = %+v, want %+v", got, want)
	}

	cloned[0].Data[1] = 'Y'
	if sourceData[1] == 'Y' {
		t.Fatal("cloned data aliases the source storage")
	}
}

func TestIncompleteTelnetCommandDoesNotPublishAnEmptyBatch(t *testing.T) {
	c := &TCPClient{inboundChan: make(chan Inbound, 1)}
	cx := &connection{
		connectionID: 4,
		parser:       NewParser(defaultCompatibility()),
		done:         make(chan struct{}),
	}

	if !c.processIncoming(cx, []byte{CmdIAC}) {
		t.Fatal("processIncoming stopped")
	}
	select {
	case inbound := <-c.Inbound():
		t.Fatalf("incomplete command published %+v", inbound)
	default:
	}

	if !c.processIncoming(cx, []byte{CmdGA}) {
		t.Fatal("processIncoming stopped after command completed")
	}
	got := <-c.Inbound()
	if len(got.Batch.Events) != 1 || got.Batch.Events[0].Kind != TelnetEventIAC || got.Batch.Events[0].Command != CmdGA {
		t.Fatalf("completed command batch = %+v", got.Batch.Events)
	}
}

func TestDataReadsRemainIncrementalAfterGA(t *testing.T) {
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
	if got := string(nextEffect(t, c, EffectServerData, "data before GA").Data); got != "HP:100> " {
		t.Fatalf("data before GA = %q", got)
	}
	nextEffect(t, c, EffectGA, "GA mark")
	advance <- struct{}{}
	if got := string(nextEffect(t, c, EffectServerData, "first data fragment").Data); got != "User" {
		t.Fatalf("first fragment = %q, want User", got)
	}
	advance <- struct{}{}
	if got := string(nextEffect(t, c, EffectServerData, "second data fragment").Data); got != "name:" {
		t.Fatalf("second fragment = %q, want name:", got)
	}
	advance <- struct{}{}
	if got := string(nextEffect(t, c, EffectServerData, "final data fragment").Data); got != " accepted\r\n" {
		t.Fatalf("final fragment = %q, want %q", got, " accepted\r\n")
	}
}

func TestBlockedLineWriteDoesNotStopIncomingParsing(t *testing.T) {
	conn := newBlockedWriteConn()
	t.Cleanup(conn.release)
	c := &TCPClient{inboundChan: make(chan Inbound, 4)}
	cx := &connection{
		conn:      conn,
		parser:    NewParser(defaultCompatibility()),
		sendQueue: make(chan outMsg, 1),
		done:      make(chan struct{}),
	}

	writeDone := make(chan bool, 1)
	go func() { writeDone <- c.writeLine(cx, []byte("look")) }()
	<-conn.writeStarted

	parseStarted := make(chan struct{})
	parseDone := make(chan bool, 1)
	go func() {
		close(parseStarted)
		parseDone <- c.processIncoming(cx, []byte("write-block-marker\r\n"))
	}()
	<-parseStarted

	select {
	case inbound := <-c.inboundChan:
		if inbound.Kind != InboundEvents || len(inbound.Batch.Events) != 1 ||
			inbound.Batch.Events[0].Kind != TelnetEventDataReceive ||
			string(inbound.Batch.Events[0].Data) != "write-block-marker\r\n" {
			t.Fatalf("incoming batch during blocked write = %+v", inbound)
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
