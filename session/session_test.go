package session

import (
	"strings"
	"testing"

	"github.com/drake/rune/event"
	"github.com/drake/rune/lua"
)

// newTestSession boots a Session against mocks with the real embedded
// core scripts, without starting Run's goroutines - tests drive
// handleEvent directly, exactly as the event loop would.
func newTestSession(t *testing.T) (*Session, *mockNetwork, *mockUI) {
	t.Helper()

	net := newMockNetwork()
	uiMock := newMockUI()
	s := New(net, uiMock, Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   t.TempDir(),
	})
	if err := s.boot(); err != nil {
		t.Fatalf("boot failed: %v", err)
	}
	uiMock.drainPrinted() // discard startup banner
	t.Cleanup(func() {
		s.timer.Stop()
	})
	return s, net, uiMock
}

func userInput(s *Session, text string) {
	s.handleEvent(event.Event{Type: event.UserInput, Payload: event.Line(text)})
}

func serverLine(s *Session, text string) {
	s.handleEvent(event.Event{Type: event.NetLine, Payload: event.Line(text)})
}

func serverPrompt(s *Session, text string) {
	s.handleEvent(event.Event{Type: event.NetPrompt, Payload: event.Line(text)})
}

func contains(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func TestInputFlowsThroughLuaToNetwork(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	userInput(s, "north")
	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "north" {
		t.Errorf("expected input to reach network, got %v", sent)
	}
}

func TestAliasExpansionEndToEnd(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("setup", `rune.alias.exact("n", "north")`); err != nil {
		t.Fatal(err)
	}
	userInput(s, "n")
	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "north" {
		t.Errorf("expected alias expansion, got %v", sent)
	}
}

func TestServerLinePrintedAndTriggersFire(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("setup", `rune.trigger.contains("attacks", "flee")`); err != nil {
		t.Fatal(err)
	}
	serverLine(s, "A goblin attacks you!")

	if printed := uiMock.drainPrinted(); !contains(printed, "goblin attacks") {
		t.Errorf("expected server line printed, got %v", printed)
	}
	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "flee" {
		t.Errorf("expected trigger to fire, got %v", sent)
	}
}

func TestGaggedLineNotPrinted(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	if err := s.engine.DoString("setup", `rune.trigger.contains("secret", nil, {gag = true})`); err != nil {
		t.Fatal(err)
	}
	serverLine(s, "a secret line")

	if printed := uiMock.drainPrinted(); contains(printed, "secret") {
		t.Errorf("gagged line was printed: %v", printed)
	}
}

// A prompt must be committed to scrollback exactly once: when input is
// submitted while it is displayed, or when a newer prompt replaces it.
func TestPromptCommitOrdering(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	serverPrompt(s, "HP:100>")
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "HP:100>" {
		t.Fatalf("expected prompt overlay set, got %v", prompts)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("prompt committed to scrollback too early: %v", printed)
	}

	// Submitting input commits the pending prompt before processing
	userInput(s, "north")
	printed := uiMock.drainPrinted()
	if !contains(printed, "HP:100>") {
		t.Errorf("expected pending prompt committed on input, got %v", printed)
	}

	// A full server line ends the prompt overlay
	serverPrompt(s, "HP:90>")
	serverLine(s, "You arrive.")
	prompts := uiMock.drainPrompts()
	if len(prompts) == 0 || prompts[len(prompts)-1] != "" {
		t.Errorf("expected prompt overlay cleared after line, got %v", prompts)
	}
}

func TestLocalEchoRespectsServerEchoMode(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	userInput(s, "hello")
	if len(uiMock.echoed) != 1 {
		t.Errorf("expected local echo, got %v", uiMock.echoed)
	}

	// Server took over echo (password entry) - no local echo
	net.localEcho = false
	userInput(s, "hunter2")
	if len(uiMock.echoed) != 1 {
		t.Errorf("expected no echo with server echo on, got %v", uiMock.echoed)
	}
}

func TestDisconnectEventUpdatesStateAndNotifiesLua(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true

	s.handleEvent(event.Event{Type: event.SysDisconnect})

	if s.clientState.Connected {
		t.Error("clientState still connected after disconnect")
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "Disconnected") {
		t.Errorf("expected disconnect notice, got %v", printed)
	}
}

// Reload must be deferred through the event queue - it tears down the
// VM that is executing the /reload command - and must leave a working
// scripting environment behind.
func TestReloadIsDeferredAndRebuildsVM(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("setup", `rune.alias.exact("n", "north")`); err != nil {
		t.Fatal(err)
	}

	s.Reload()

	// The reload callback is queued, not executed inline
	select {
	case ev := <-s.events:
		if ev.Type != event.AsyncResult {
			t.Fatalf("expected AsyncResult, got %v", ev.Type)
		}
		s.handleEvent(ev)
	default:
		t.Fatal("reload did not queue an event")
	}

	if printed := uiMock.drainPrinted(); !contains(printed, "Scripts reloaded") {
		t.Errorf("expected reload completion notice, got %v", printed)
	}

	// The old VM's registrations are gone; the new VM works
	userInput(s, "n")
	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "n" {
		t.Errorf("expected alias gone after reload, got %v", sent)
	}
	if err := s.engine.DoString("check", `assert(rune.hooks ~= nil)`); err != nil {
		t.Errorf("scripting broken after reload: %v", err)
	}
}

func TestSlashCommandDoesNotReachServer(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	userInput(s, "/help")
	if sent := net.drainSent(); len(sent) != 0 {
		t.Errorf("slash command leaked to server: %v", sent)
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "/connect") {
		t.Errorf("expected help output, got %v", printed)
	}
}

func TestHistoryDedupAndTrim(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.historyLimit = 3

	for _, cmd := range []string{"a", "a", "b", "", "c", "d"} {
		s.AddToHistory(cmd)
	}
	got := s.GetHistory()
	want := []string{"b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("history = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history = %v, want %v", got, want)
		}
	}
}

func TestSendFailureReportedNotFatal(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = false // sends fail

	userInput(s, "north")
	if printed := uiMock.drainPrinted(); !contains(printed, "not connected") {
		t.Errorf("expected send failure echoed, got %v", printed)
	}
}
