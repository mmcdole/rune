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
	"math/big"
	"net"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, "tls+insecure://"+ln.Addr().String()); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, "tls://"+ln.Addr().String()); err == nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, addr); err != nil {
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

// nextOutput waits for the next Output of the given kind, skipping
// unrelated events (the output buffer emits partial snapshots freely).
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

// --- Identity negotiation (Phase 1) over a real socket ---

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, addr); err != nil {
		t.Fatalf("connect: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("server script did not complete")
	}
}

// --- MCCP2 (Phase 2) ---

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

// --- GMCP (Phase 3) ---

func TestGMCPLoopback(t *testing.T) {
	fromServer := make(chan []byte, 1)
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte{CmdIAC, CmdWILL, OptGMCP})
		expectBytes(t, conn, []byte{CmdIAC, CmdDO, OptGMCP}, "DO GMCP")

		conn.Write(subnegFrame(OptGMCP, []byte(`Char.Vitals {"hp":100,"maxhp":200}`)))

		// Then wait for the client's own GMCP message
		want := subnegFrame(OptGMCP, []byte(`Core.Hello {"client":"Rune"}`))
		got := expectBytes(t, conn, want, "client Core.Hello frame")
		fromServer <- got
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
	select {
	case <-fromServer:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received the client's GMCP frame")
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

// --- prompt emission ---

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

// TestUnterminatedPromptSurvivesPromptlessNegotiation pins the
// prompt-mode policy: negotiating SGA or EOR is not evidence of prompt
// termination (WILL is a promise, DO concerns our output), so a server
// that negotiates either but never sends GA/EOR keeps its unterminated
// prompts visible.
func TestUnterminatedPromptSurvivesPromptlessNegotiation(t *testing.T) {
	for _, neg := range []struct {
		name  string
		bytes []byte
	}{
		{"WILL SGA", []byte{CmdIAC, CmdWILL, OptSGA}},
		{"DO EOR", []byte{CmdIAC, CmdDO, OptEOR}},
	} {
		t.Run(neg.name, func(t *testing.T) {
			addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
				conn.Write(neg.bytes)
				time.Sleep(50 * time.Millisecond)
				conn.Write([]byte("Enter your name: "))

				buf := make([]byte, 1)
				conn.SetReadDeadline(time.Now().Add(10 * time.Second))
				conn.Read(buf)
			})

			c := connectLoopback(t, addr)
			out := nextOutput(t, c, OutputPartial, "unterminated prompt after "+neg.name)
			if out.Payload != "Enter your name: " {
				t.Fatalf("prompt = %q, want %q", out.Payload, "Enter your name: ")
			}
		})
	}
}

// A server may use GA/EOR for some prompts and omit it for others. A received
// marker must not permanently hide later unfinished text.
func TestPartialSnapshotsContinueAfterGA(t *testing.T) {
	advance := make(chan struct{})
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write(append([]byte("HP:100> "), CmdIAC, CmdGA))
		<-advance
		conn.Write([]byte("HP:90> ")) // partial, terminator still in flight
		<-advance
		conn.Write([]byte{CmdIAC, CmdGA})
		conn.Write([]byte("marker\r\n"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	var partials, prompts []string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputPartial:
				partials = append(partials, out.Payload)
				if out.Payload == "HP:90> " {
					advance <- struct{}{}
				}
			case OutputPrompt:
				if out.PromptTerminator != PromptTerminatorGA {
					t.Fatalf("prompt %q terminator = %v, want GA", out.Payload, out.PromptTerminator)
				}
				prompts = append(prompts, out.Payload)
				if out.Payload == "HP:100> " {
					advance <- struct{}{}
				}
			case OutputLine:
				if out.Payload == "marker" {
					if len(partials) != 1 || partials[0] != "HP:90> " {
						t.Fatalf("partial snapshots = %q, want [\"HP:90> \"]", partials)
					}
					if len(prompts) != 2 || prompts[0] != "HP:100> " || prompts[1] != "HP:90> " {
						t.Fatalf("confirmed prompts = %q", prompts)
					}
					return
				}
			case OutputDisconnect:
				t.Fatal("connection dropped while waiting for marker")
			}
		case <-deadline:
			t.Fatal("timed out waiting for marker line")
		}
	}
}

// TestPromptEmittedOncePerGABatch pins the duplicate-prompt bug: a
// line and a GA-terminated prompt arriving in one read must produce
// exactly one prompt event. Before the fix, the unterminated-mode
// peek fired per data event and the GA flush fired again, so the same
// prompt went out twice and the session committed the duplicate.
func TestPromptEmittedOncePerGABatch(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		// Line + partial prompt + IAC GA in a single write.
		conn.Write(append([]byte("the sun rises\r\nHP:100> "), CmdIAC, CmdGA))
		// Marker line: once the client emits it, everything above has
		// been fully processed.
		conn.Write([]byte("marker\r\n"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)

	var prompts []string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputPartial:
				t.Fatalf("same-batch GA prompt emitted a preliminary partial: %q", out.Payload)
			case OutputPrompt:
				if out.PromptTerminator != PromptTerminatorGA {
					t.Fatalf("prompt %q terminator = %v, want GA", out.Payload, out.PromptTerminator)
				}
				prompts = append(prompts, out.Payload)
			case OutputLine:
				if out.Payload == "marker" {
					if len(prompts) != 1 || prompts[0] != "HP:100> " {
						t.Fatalf("prompt events = %q, want exactly [\"HP:100> \"]", prompts)
					}
					return
				}
			case OutputDisconnect:
				t.Fatal("connection dropped while waiting for marker")
			}
		case <-deadline:
			t.Fatal("timed out waiting for marker line")
		}
	}
}

func TestPartialGrowsThenCompletesAsOneLine(t *testing.T) {
	advance := make(chan struct{})
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
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
	if got := nextOutput(t, c, OutputPartial, "first partial").Payload; got != "User" {
		t.Fatalf("first partial = %q, want User", got)
	}
	advance <- struct{}{}
	if got := nextOutput(t, c, OutputPartial, "grown partial").Payload; got != "Username:" {
		t.Fatalf("grown partial = %q, want Username:", got)
	}
	advance <- struct{}{}
	if got := nextOutput(t, c, OutputLine, "completed line").Payload; got != "Username: accepted" {
		t.Fatalf("completed line = %q, want %q", got, "Username: accepted")
	}
}

func TestPromptPreservesGAVersusEOR(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  byte
		want PromptTerminator
	}{
		{name: "GA", cmd: CmdGA, want: PromptTerminatorGA},
		{name: "EOR", cmd: CmdEOR, want: PromptTerminatorEOR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
				conn.Write(append([]byte("HP:100> "), CmdIAC, tc.cmd))
				buf := make([]byte, 1)
				conn.SetReadDeadline(time.Now().Add(10 * time.Second))
				conn.Read(buf)
			})

			c := connectLoopback(t, addr)
			out := nextOutput(t, c, OutputPrompt, tc.name+" prompt")
			if out.Payload != "HP:100> " || out.PromptTerminator != tc.want {
				t.Fatalf("prompt = (%q, %v), want (%q, %v)",
					out.Payload, out.PromptTerminator, "HP:100> ", tc.want)
			}
		})
	}
}

func TestSentLineSeparatesUnterminatedPrompts(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte("Username: "))
		expectBytes(t, conn, []byte("player\r\n"), "login name")
		conn.Write([]byte("Password: "))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	if got := nextOutput(t, c, OutputPartial, "username prompt").Payload; got != "Username: " {
		t.Fatalf("first partial = %q", got)
	}
	if err := c.Send("player"); err != nil {
		t.Fatal(err)
	}
	if got := nextOutput(t, c, OutputPartial, "password prompt").Payload; got != "Password: " {
		t.Fatalf("second partial = %q, want Password without the old tail", got)
	}
}

func TestSentLineEndsPartialEpochBeforeResponse(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte("first half"))
		expectBytes(t, conn, []byte("look\r\n"), "command after partial")
		conn.Write([]byte(" second half\r\nnext tail"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	first := nextOutput(t, c, OutputPartial, "first partial")
	if first.Payload != "first half" || first.PartialEpoch == 0 {
		t.Fatalf("first partial = (%q, epoch %d)", first.Payload, first.PartialEpoch)
	}
	if err := c.Send("look"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	sawBoundary := false
	sawTruncatedLine := false
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputSendBoundary:
				if sawBoundary {
					t.Fatal("duplicate send boundary")
				}
				if out.PartialEpoch != first.PartialEpoch {
					t.Fatalf("boundary epoch = %d, want %d", out.PartialEpoch, first.PartialEpoch)
				}
				sawBoundary = true
			case OutputLine:
				if !sawBoundary {
					t.Fatalf("response line overtook send boundary: %q", out.Payload)
				}
				if out.Payload != " second half" {
					t.Fatalf("line after mid-fragment send = %q", out.Payload)
				}
				sawTruncatedLine = true
			case OutputPartial:
				if !sawTruncatedLine {
					t.Fatalf("new partial arrived before response line: %q", out.Payload)
				}
				if out.Payload != "next tail" {
					t.Fatalf("new partial = %q", out.Payload)
				}
				if out.PartialEpoch != first.PartialEpoch+1 {
					t.Fatalf("new partial epoch = %d, want %d", out.PartialEpoch, first.PartialEpoch+1)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for boundary and post-command output")
		}
	}
}

func TestSentLineEmitsBoundaryAfterConsumedPrompt(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write(append([]byte("HP:100> "), CmdIAC, CmdGA))
		expectBytes(t, conn, []byte("look\r\n"), "command after confirmed prompt")
		conn.Write([]byte("marker\r\n"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	prompt := nextOutput(t, c, OutputPrompt, "confirmed prompt")
	if prompt.PartialEpoch == 0 {
		t.Fatal("confirmed prompt did not carry its partial epoch")
	}
	if err := c.Send("look"); err != nil {
		t.Fatal(err)
	}
	boundary := nextOutput(t, c, OutputSendBoundary, "send boundary")
	if boundary.PartialEpoch != prompt.PartialEpoch {
		t.Fatalf("boundary epoch = %d, want prompt epoch %d", boundary.PartialEpoch, prompt.PartialEpoch)
	}
	if got := nextOutput(t, c, OutputLine, "response line").Payload; got != "marker" {
		t.Fatalf("response line = %q", got)
	}
}

func TestBareCRBeforeGABecomesLine(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		wire := append([]byte("complete line\r"), CmdIAC, CmdGA)
		wire = append(wire, []byte("\nmarker\r\n")...)
		conn.Write(wire)

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	var lines []string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputPrompt:
				t.Fatalf("bare-CR line was misclassified as prompt: %q", out.Payload)
			case OutputLine:
				lines = append(lines, out.Payload)
				if out.Payload == "marker" {
					if len(lines) != 2 || lines[0] != "complete line" {
						t.Fatalf("lines = %q", lines)
					}
					return
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for bare-CR line")
		}
	}
}

func TestEmptyGAAndEORDoNotEmitTextEvents(t *testing.T) {
	addr := telnetServer(t, func(t *testing.T, conn net.Conn) {
		conn.Write([]byte{CmdIAC, CmdGA, CmdIAC, CmdEOR})
		conn.Write([]byte("marker\r\n"))

		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		conn.Read(buf)
	})

	c := connectLoopback(t, addr)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case out := <-c.Output():
			switch out.Kind {
			case OutputPrompt, OutputPartial:
				t.Fatalf("empty GA/EOR emitted text event: kind=%v payload=%q", out.Kind, out.Payload)
			case OutputLine:
				if out.Payload == "marker" {
					return
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for marker line")
		}
	}
}
