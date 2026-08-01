package session

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

// newTestSession boots a Session against mocks with the real embedded
// core scripts, without starting Run's goroutines - tests call the
// same handlers the event loop dispatches to, synchronously.
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
	s.handleSubmission(input.Command(text))
}

func serverLine(s *Session, text string) {
	s.handleNetworkOutput(network.Output{Kind: network.OutputLine, Payload: text})
}

func serverPrompt(s *Session, text string) {
	s.handleNetworkOutput(network.Output{Kind: network.OutputPrompt, Payload: text})
}

func contains(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// Stimulus/response flows (input->network, aliases, triggers, gags,
// local echo, slash commands) are covered end-to-end by the scenario
// suite in test/e2e/scenarios/. The tests here assert synchronous internals
// the scenario vocabulary cannot express.

// A prompt must be committed to scrollback exactly once, when input is
// submitted while it is displayed. New prompt snapshots replace the overlay,
// and a completed server line clears it without committing it.
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

	// A growing unterminated line produces cumulative prompt snapshots. The
	// latest snapshot replaces the overlay without committing the earlier one.
	serverPrompt(s, "HP:100> ready")
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "HP:100> ready" {
		t.Fatalf("expected updated prompt overlay, got %v", prompts)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("superseded prompt snapshot committed to scrollback: %v", printed)
	}

	// Submitting input commits only the latest pending prompt before processing.
	userInput(s, "north")
	printed := uiMock.drainPrinted()
	promptCount := 0
	for _, line := range printed {
		if line == "HP:100>" {
			t.Errorf("superseded prompt snapshot committed on input: %v", printed)
		}
		if line == "HP:100> ready" {
			promptCount++
		}
	}
	if promptCount != 1 {
		t.Errorf("latest prompt committed %d times on input, want 1; got %v", promptCount, printed)
	}

	// A full server line ends the prompt overlay
	serverPrompt(s, "HP:90>")
	serverLine(s, "You arrive.")
	prompts := uiMock.drainPrompts()
	if len(prompts) == 0 || prompts[len(prompts)-1] != "" {
		t.Errorf("expected prompt overlay cleared after line, got %v", prompts)
	}
}

func TestDisconnectEventUpdatesStateAndNotifiesLua(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true

	s.handleNetworkOutput(network.Output{Kind: network.OutputDisconnect})

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
	case cb := <-s.asyncResults:
		cb()
	default:
		t.Fatal("reload did not queue a callback")
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

func TestHistoryPreservesModeAndDedupesWholeSubmission(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.historyLimit = 4

	for _, entry := range []input.Submission{
		input.Command("same"),
		input.Command("same"), // exact adjacent duplicate
		input.Verbatim("same"),
		input.Verbatim("same"), // exact adjacent duplicate
		input.Command("next"),
	} {
		s.addHistorySubmission(entry)
	}

	want := []input.Submission{
		input.Command("same"),
		input.Verbatim("same"),
		input.Command("next"),
	}
	got := s.GetHistoryEntries()
	if len(got) != len(want) {
		t.Fatalf("structured history = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("structured history[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// The compatibility API deliberately projects both differently-modeled
	// entries to strings, even when that makes adjacent text look duplicated.
	legacy := s.GetHistory()
	if got, want := strings.Join(legacy, "|"), "same|same|next"; got != want {
		t.Fatalf("legacy history = %q, want %q", got, want)
	}

	// Callers receive a copy, not Session's canonical backing slice.
	got[0] = input.Command("mutated")
	if s.GetHistoryEntries()[0].Text != "same" {
		t.Fatal("GetHistoryEntries exposed mutable Session storage")
	}
}

func TestSetInputSubmissionForwardsExplicitMode(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	want := input.Verbatim("café;still data")

	s.SetInputSubmission(want)

	if len(uiMock.inputModes) != 1 || uiMock.inputModes[0] != want {
		t.Fatalf("explicit input updates = %+v, want [%+v]", uiMock.inputModes, want)
	}
	if got := s.GetInput(); got != want.Text {
		t.Fatalf("Session input mirror = %q, want %q", got, want.Text)
	}
	if got, wantCursor := s.InputGetCursor(), len(want.Text); got != wantCursor {
		t.Fatalf("Session cursor mirror = %d, want %d", got, wantCursor)
	}
}

func TestInputCursorConvertsAtUIBoundary(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	s.handleUIMessage(ui.InputChangedMsg{Text: "café gob", Cursor: 8})
	if got, want := s.InputGetCursor(), len("café gob"); got != want {
		t.Fatalf("cursor after input change = %d, want %d", got, want)
	}

	s.handleUIMessage(ui.CursorMovedMsg{Cursor: 4})
	if got, want := s.InputGetCursor(), len("café"); got != want {
		t.Fatalf("cursor after UI move = %d, want %d", got, want)
	}

	s.InputSetCursor(4)
	if got, want := s.InputGetCursor(), 3; got != want {
		t.Fatalf("cursor inside UTF-8 sequence = %d, want %d", got, want)
	}
	if got, want := uiMock.inputCursor[len(uiMock.inputCursor)-1], 3; got != want {
		t.Fatalf("widget cursor = %d, want %d", got, want)
	}

	s.InputSetCursor(len("café"))
	if got, want := uiMock.inputCursor[len(uiMock.inputCursor)-1], 4; got != want {
		t.Fatalf("widget cursor after multibyte text = %d, want %d", got, want)
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

func TestVerbatimSubmissionPreservesPhysicalLines(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	text := "  say hi;look  \n\t#2 north\n\n/quit\ntrailing  "
	s.handleSubmission(input.Verbatim(text))

	want := []string{"  say hi;look  ", "\t#2 north", "", "/quit", "trailing  "}
	got := net.drainSent()
	if len(got) != len(want) {
		t.Fatalf("sent %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sent[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if history := s.GetHistory(); len(history) != 1 || history[0] != text {
		t.Fatalf("history = %q, want one exact submission %q", history, text)
	}
	if history := s.GetHistoryEntries(); len(history) != 1 || history[0] != input.Verbatim(text) {
		t.Fatalf("structured history = %+v, want one verbatim submission", history)
	}
	for _, echoed := range uiMock.drainEchoed() {
		if strings.ContainsRune(echoed, '\n') {
			t.Fatalf("echo contains embedded newline: %q", echoed)
		}
	}
	select {
	case <-uiMock.done:
		t.Fatal("verbatim /quit was interpreted as a client command")
	default:
	}
}

func TestSubmissionEchoVisualizesControlsWithoutChangingWireData(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	raw := "safe\x1b]52;c;payload\a\tend\nnext\x00"
	s.handleSubmission(input.Verbatim(raw))

	wantSent := []string{"safe\x1b]52;c;payload\a\tend", "next\x00"}
	if got := net.drainSent(); len(got) != len(wantSent) || got[0] != wantSent[0] || got[1] != wantSent[1] {
		t.Fatalf("wire data = %q, want exact %q", got, wantSent)
	}

	echoed := uiMock.drainEchoed()
	if len(echoed) != 2 {
		t.Fatalf("echoed %d lines, want 2: %q", len(echoed), echoed)
	}
	plain := runetext.StripANSI(strings.Join(echoed, "\n"))
	for _, want := range []string{"␛]52", "␇", "\t", "␀"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("safe echo missing %q: %q", want, plain)
		}
	}
}
