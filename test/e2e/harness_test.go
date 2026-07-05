package e2e

// End-to-end harness: a real Session.Run event loop driving a real
// network.TCPClient against a scripted TCP server, with the mock UI
// as the only test double. Scenario JSON files in scenarios/ run on
// top of this via runner_test.go; imperative Go tests in this package
// (none yet) use it directly for cases the step vocabulary cannot
// express.

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/session"
	"github.com/mmcdole/rune/ui"
)

// Telnet bytes used by the scripted server (the network package keeps
// its constants unexported).
const (
	bIAC  = 255
	bSE   = 240
	bGA   = 249
	bSB   = 250
	bWILL = 251
	bDO   = 253
	bNAWS = 31
	bGMCP = 201
)

const waitTimeout = 5 * time.Second

// --- fake MUD server ---

// fakeMUD is a scripted server: tests write protocol bytes at it and
// assert on what the client puts on the wire.
type fakeMUD struct {
	t    *testing.T
	ln   net.Listener
	conn net.Conn
	got  []byte // everything read from the client so far
}

func newFakeMUD(t *testing.T) *fakeMUD {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := &fakeMUD{t: t, ln: ln}
	t.Cleanup(func() {
		ln.Close()
		if m.conn != nil {
			m.conn.Close()
		}
	})
	return m
}

func (m *fakeMUD) addr() string { return m.ln.Addr().String() }

// accept blocks until the client dials in. Reusable: the listener
// stays open, so a later "accept" step picks up a reconnection (the
// previous conn is replaced; the wire capture keeps accumulating).
func (m *fakeMUD) accept() {
	m.t.Helper()
	if tcp, ok := m.ln.(*net.TCPListener); ok {
		tcp.SetDeadline(time.Now().Add(waitTimeout))
	}
	conn, err := m.ln.Accept()
	if err != nil {
		m.t.Fatalf("client never dialed the fake MUD: %v", err)
	}
	if m.conn != nil {
		m.conn.Close()
	}
	m.conn = conn
}

func (m *fakeMUD) write(b []byte) {
	m.t.Helper()
	if m.conn == nil {
		m.t.Fatal("fake MUD write before connect step")
	}
	if _, err := m.conn.Write(b); err != nil {
		m.t.Fatalf("fake MUD write: %v", err)
	}
}

func (m *fakeMUD) writeLine(s string) { m.write([]byte(s + "\r\n")) }

// writePrompt sends a partial line terminated by IAC GA.
func (m *fakeMUD) writePrompt(s string) { m.write(append([]byte(s), bIAC, bGA)) }

func (m *fakeMUD) writeGMCP(payload string) {
	frame := append([]byte{bIAC, bSB, bGMCP}, []byte(payload)...)
	m.write(append(frame, bIAC, bSE))
}

// expect reads from the client until want appears in the accumulated
// stream. Reads accumulate across calls, so earlier bytes stay visible.
func (m *fakeMUD) expect(want []byte, what string) {
	m.t.Helper()
	if m.conn == nil {
		m.t.Fatalf("%s: no connection (missing connect step?)", what)
	}
	m.conn.SetReadDeadline(time.Now().Add(waitTimeout))
	defer m.conn.SetReadDeadline(time.Time{})
	buf := make([]byte, 512)
	for !bytes.Contains(m.got, want) {
		n, err := m.conn.Read(buf)
		if n > 0 {
			m.got = append(m.got, buf[:n]...)
		}
		if err != nil {
			m.t.Fatalf("%s: wanted %q in client stream, got %q (read error: %v)", what, want, m.got, err)
		}
	}
}

// sent reports whether the wire stream so far contains want, without
// waiting for more bytes.
func (m *fakeMUD) sent(want []byte) bool {
	return bytes.Contains(m.got, want)
}

// readAnything reports whether any client bytes have been read yet.
// The capture only grows inside expect, so a negative wire check
// before the first positive one would pass vacuously.
func (m *fakeMUD) readAnything() bool {
	return len(m.got) > 0
}

// --- mock UI ---

// mockUI implements ui.UI, capturing display calls for assertions.
type mockUI struct {
	mu       sync.Mutex
	printed  []string
	echoed   []string
	prompts  []string // every SetPrompt call, including clears
	inputSet []string
	input    chan string
	outbound chan ui.UIEvent
	done     chan struct{}
}

var _ ui.UI = (*mockUI)(nil)

func newMockUI() *mockUI {
	return &mockUI{
		input:    make(chan string, 64),
		outbound: make(chan ui.UIEvent, 64),
		done:     make(chan struct{}),
	}
}

func (m *mockUI) Run() error { <-m.done; return nil }
func (m *mockUI) Quit() {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}
func (m *mockUI) Input() <-chan string        { return m.input }
func (m *mockUI) Outbound() <-chan ui.UIEvent { return m.outbound }

func (m *mockUI) Print(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.printed = append(m.printed, text)
}

func (m *mockUI) Echo(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.echoed = append(m.echoed, text)
}

func (m *mockUI) SetPrompt(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, text)
}

func (m *mockUI) SetInput(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputSet = append(m.inputSet, text)
}

func (m *mockUI) UpdateBars(content map[string]ui.BarContent) {}
func (m *mockUI) UpdateBinds(keys map[string]bool)            {}
func (m *mockUI) UpdateLayout(top, bottom []ui.LayoutEntry)   {}
func (m *mockUI) ShowPicker(opts ui.ShowPickerMsg)            {}
func (m *mockUI) CreatePane(name string)                      {}
func (m *mockUI) WritePane(name, text string)                 {}
func (m *mockUI) TogglePane(name string)                      {}
func (m *mockUI) ClearPane(name string)                       {}
func (m *mockUI) InputSetCursor(pos int)                      {}
func (m *mockUI) OpenEditor(initial string) (string, bool)    { return "", false }
func (m *mockUI) PaneScrollUp(name string, lines int)         {}
func (m *mockUI) PaneScrollDown(name string, lines int)       {}
func (m *mockUI) PaneScrollToTop(name string)                 {}
func (m *mockUI) PaneScrollToBottom(name string)              {}

func (m *mockUI) printedContains(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return containsSubstr(m.printed, substr)
}

func (m *mockUI) printedSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.printed...)
}

func (m *mockUI) echoedContains(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return containsSubstr(m.echoed, substr)
}

func (m *mockUI) promptContains(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return containsSubstr(m.prompts, substr)
}

func (m *mockUI) lastInputSet() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.inputSet) == 0 {
		return "", false
	}
	return m.inputSet[len(m.inputSet)-1], true
}

func containsSubstr(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// --- client under test ---

// client is one running rune instance wired to a fake MUD.
type client struct {
	t   *testing.T
	ui  *mockUI
	mud *fakeMUD
}

// newClient boots a real Session with a live event loop and a fake
// MUD ready to be dialed. Shutdown is verified in cleanup: every test
// implicitly asserts the client quits cleanly.
func newClient(t *testing.T, initLua string) *client {
	t.Helper()

	cfgDir := t.TempDir()
	if initLua != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "init.lua"), []byte(initLua), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	uiMock := newMockUI()
	s := session.New(network.NewTCPClient(), uiMock, session.Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   cfgDir,
	})

	done := make(chan struct{})
	var runErr error
	go func() {
		runErr = s.Run(context.Background())
		close(done)
	}()

	t.Cleanup(func() {
		uiMock.Quit()
		select {
		case <-done:
			if runErr != nil {
				t.Errorf("Session.Run returned error: %v", runErr)
			}
		case <-time.After(10 * time.Second):
			t.Error("Session.Run did not shut down within 10s")
		}
	})

	return &client{t: t, ui: uiMock, mud: newFakeMUD(t)}
}

// connect types /connect at the client and waits for the dial.
func (c *client) connect() {
	c.t.Helper()
	c.ui.input <- "/connect " + c.mud.addr()
	c.mud.accept()
}

// connectRefused closes the fake MUD's listener and then types
// /connect at the now-dead address, so the dial is refused.
func (c *client) connectRefused() {
	c.t.Helper()
	addr := c.mud.addr()
	c.mud.ln.Close()
	c.ui.input <- "/connect " + addr
}

// waitFor polls cond until it holds or the deadline passes.
func (c *client) waitFor(what string, cond func() bool) {
	c.t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.t.Fatalf("timed out waiting for %s", what)
}
