package session

import (
	"slices"
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
		s.LogStop()
		s.timer.Stop()
	})
	return s, net, uiMock
}

func userInput(s *Session, text string) {
	s.handleSubmission(input.Command(text))
}

func serverLine(s *Session, text string) {
	s.handleNetworkOutput(network.Output{Kind: network.OutputLine, ConnectionID: s.connectionID, Payload: text})
}

func serverPartial(s *Session, text string) {
	s.handleNetworkOutput(network.Output{Kind: network.OutputPartial, ConnectionID: s.connectionID, Payload: text})
}

func serverPrompt(s *Session, text string) {
	s.handleNetworkOutput(network.Output{Kind: network.OutputPrompt, ConnectionID: s.connectionID, Payload: text})
}

func sendBoundary(s *Session) {
	s.handleNetworkOutput(network.Output{Kind: network.OutputSendBoundary, ConnectionID: s.connectionID})
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

// A prompt overlay is committed only when the writer publishes a send boundary.
func TestPromptOverlayCommitOrdering(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	serverPartial(s, "HP:100>")
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "HP:100>" {
		t.Fatalf("expected prompt overlay set, got %v", prompts)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("prompt committed to scrollback too early: %v", printed)
	}

	// A growing partial line replaces the prompt overlay in place.
	serverPartial(s, "HP:100> ready")
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "HP:100> ready" {
		t.Fatalf("expected updated prompt overlay, got %v", prompts)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("superseded partial line committed to scrollback: %v", printed)
	}

	// Submitting input does not commit the overlay by itself.
	userInput(s, "north")
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("submission committed prompt before network send: %v", printed)
	}
	if prompts := uiMock.drainPrompts(); len(prompts) != 0 {
		t.Fatalf("submission repainted prompt before network send: %v", prompts)
	}

	// The send boundary commits only the latest partial line.
	sendBoundary(s)
	printed := uiMock.drainPrinted()
	promptCount := 0
	for _, line := range printed {
		if line == "HP:100>" {
			t.Errorf("superseded partial line committed at send boundary: %v", printed)
		}
		if line == "HP:100> ready" {
			promptCount++
		}
	}
	if promptCount != 1 {
		t.Errorf("latest partial line committed %d times, want 1; got %v", promptCount, printed)
	}

	// A full server line ends the prompt overlay
	serverPartial(s, "HP:90>")
	serverLine(s, "You arrive.")
	prompts := uiMock.drainPrompts()
	if len(prompts) == 0 || prompts[len(prompts)-1] != "" {
		t.Errorf("expected prompt overlay cleared after line, got %v", prompts)
	}
}

func TestConfirmedPromptCommitsBeforeFollowingServerText(t *testing.T) {
	tests := []struct {
		name string
		next func(*Session)
		want []string
	}{
		{
			name: "complete line",
			next: func(s *Session) { serverLine(s, "A bell rings.") },
			want: []string{"HP:100>", "A bell rings."},
		},
		{
			name: "partial line",
			next: func(s *Session) { serverPartial(s, "A bell") },
			want: []string{"HP:100>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, uiMock := newTestSession(t)
			serverPrompt(s, "HP:100>")
			uiMock.drainPrompts()
			if printed := uiMock.drainPrinted(); len(printed) != 0 {
				t.Fatalf("confirmed prompt committed before following text: %v", printed)
			}

			tt.next(s)
			if printed := uiMock.drainPrinted(); !slices.Equal(printed, tt.want) {
				t.Fatalf("printed = %q, want %q", printed, tt.want)
			}
		})
	}
}

func TestConnectionChangeDiscardsPromptOverlayAndSpans(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Session)
	}{
		{name: "connect", change: func(s *Session) { s.Connect("example.test:4000") }},
		{name: "disconnect", change: func(s *Session) { s.Disconnect() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = true
			if err := s.engine.DoString("open old connection span", `
				fired = 0
				rune.trigger.starts("Old session:", function()
					fired = fired + 1
				end, { gag = true, span = { to = "NEVER", max = 8 } })
			`); err != nil {
				t.Fatal(err)
			}

			serverLine(s, "Old session: unfinished")
			uiMock.drainPrinted()
			serverPartial(s, "Username:")
			uiMock.drainPrompts()
			tt.change(s)

			if contains(uiMock.drainPrinted(), "Username:") {
				t.Fatal("connection change committed stale prompt overlay")
			}
			if prompts := uiMock.drainPrompts(); len(prompts) == 0 || prompts[len(prompts)-1] != "" {
				t.Fatalf("connection change did not clear prompt overlay: %q", prompts)
			}
			serverLine(s, "New session output")
			if printed := uiMock.drainPrinted(); !contains(printed, "New session output") {
				t.Fatalf("discarded span consumed new connection output: %q", printed)
			}
			if err := s.engine.DoString("assert", `assert(fired == 0, "discard fired old span")`); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPromptHookDisconnectDoesNotRestorePreviousPrompt(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("disconnect from prompt", `
		rune.hooks.on("prompt", function()
			rune.disconnect()
		end, { priority = 10, once = true })
	`); err != nil {
		t.Fatal(err)
	}

	serverPartial(s, "previous Username:")
	if net.connected {
		t.Fatal("prompt hook did not disconnect")
	}
	if s.prompt.active {
		t.Fatal("outer prompt handler restored the previous prompt overlay")
	}
	prompts := uiMock.drainPrompts()
	if len(prompts) == 0 || prompts[len(prompts)-1] != "" {
		t.Fatalf("previous prompt overlay was repainted after disconnect: %q", prompts)
	}
}

func TestStaleSendBoundaryCannotCommitNewConnectionPrompt(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	oldConnection := s.connectionID
	s.Disconnect() // advance the connection ID and clear the old prompt
	uiMock.drainPrinted()
	uiMock.drainPrompts()
	serverPartial(s, "new Username:")
	uiMock.drainPrompts()

	s.handleNetworkOutput(network.Output{Kind: network.OutputSendBoundary, ConnectionID: oldConnection})
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("old boundary committed new connection prompt: %q", printed)
	}
	if !s.prompt.active || s.prompt.text != "new Username:" {
		t.Fatalf("old boundary changed new prompt: %+v", s.prompt)
	}
}

func TestSendBoundaryCommitsPartialLineAndFlushesSpan(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	if err := s.engine.DoString("open span", `
		fired = 0
		rune.trigger.starts("Story:", function()
			fired = fired + 1
		end, { span = { to = "NEVER", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverPartial(s, "Tundra tells you: meet me at the")
	uiMock.drainPrompts()

	sendBoundary(s)

	if printed := uiMock.drainPrinted(); len(printed) != 1 || printed[0] != "Tundra tells you: meet me at the" {
		t.Fatalf("send boundary did not commit partial line: %q", printed)
	}
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "" {
		t.Fatalf("send boundary did not clear overlay: %q", prompts)
	}
	if err := s.engine.DoString("assert", `assert(fired == 1, "send boundary did not flush span")`); err != nil {
		t.Fatal(err)
	}
}

// The send boundary arrives after prompt hooks finish, so it commits the final
// rewrite rather than the text that first matched.
func TestPromptTriggerSendCommitsFinalRewrite(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("prompt action and later rewrite", `
		rune.trigger.exact("Username:", "player", { on = "prompt" })
		rune.hooks.on("prompt", function(line)
			if line:clean() == "Username:" then
				return "Final login prompt"
			end
		end, { priority = 200 })
	`); err != nil {
		t.Fatal(err)
	}

	serverPartial(s, "Username:")

	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "player" {
		t.Fatalf("prompt action sent %q, want player", sent)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("prompt committed before its ordered boundary: %q", printed)
	}
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "Final login prompt" {
		t.Fatalf("final prompt rewrite = %q", prompts)
	}

	sendBoundary(s)
	if printed := uiMock.drainPrinted(); len(printed) != 1 || printed[0] != "Final login prompt" {
		t.Fatalf("boundary committed %q, want final prompt rewrite", printed)
	}
}

func TestSendBoundaryWithoutPromptDoesNotFlushOpenSpan(t *testing.T) {
	s, _, _ := newTestSession(t)

	if err := s.engine.DoString("open span", `
		fired = 0
		got_text = nil
		rune.trigger.starts("Story:", function(matches, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "Story: unfinished")
	sendBoundary(s)
	if err := s.engine.DoString("assert", `assert(fired == 0, "unrelated send flushed span")`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "continuation END")
	if err := s.engine.DoString("assert", `
		assert(fired == 1, "span did not finish")
		assert(got_text == "Story: unfinished continuation END",
			"span was truncated: " .. tostring(got_text))
	`); err != nil {
		t.Fatal(err)
	}
}

func TestSendBoundaryClosesSpanWithoutPrintingGaggedPartial(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	if err := s.engine.DoString("span and prompt gag", `
		fired = 0
		rune.trigger.starts("Story:", function(matches, ctx)
			fired = fired + 1
		end, { span = { to = "NEVER", max = 8 } })
		rune.trigger.exact("Password:", nil, { on = "prompt", gag = true })
	`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverPartial(s, "Password:")
	if prompts := uiMock.drainPrompts(); len(prompts) == 0 || prompts[len(prompts)-1] != "" {
		t.Fatalf("gagged partial should leave an empty overlay, got %q", prompts)
	}

	sendBoundary(s)
	if err := s.engine.DoString("assert", `assert(fired == 1, "send boundary did not flush span")`); err != nil {
		t.Fatal(err)
	}
	if contains(uiMock.drainPrinted(), "Password:") {
		t.Fatal("gagged partial was committed to scrollback")
	}
}

func TestLocalSubmissionLeavesPromptOverlayAndSpanOpen(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("open span", `
		fired = 0
		got_text = nil
		rune.trigger.starts("Story:", function(matches, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverPartial(s, "Username:")
	uiMock.drainPrompts()

	userInput(s, "/help")
	if got := uiMock.drainEchoQueuedLines(); len(got) != 1 || got[0] {
		t.Fatalf("local submission queued-line signal = %v, want [false]", got)
	}
	if sent := net.drainSent(); len(sent) != 0 {
		t.Fatalf("local command unexpectedly reached network: %q", sent)
	}
	if contains(uiMock.drainPrinted(), "Username:") {
		t.Fatal("local command committed prompt overlay")
	}
	if prompts := uiMock.drainPrompts(); len(prompts) != 0 {
		t.Fatalf("local command repainted prompt overlay: %q", prompts)
	}
	if err := s.engine.DoString("assert", `assert(fired == 0, "local command flushed span")`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "continuation END")
	if err := s.engine.DoString("assert", `
		assert(fired == 1, "span did not finish after local command")
		assert(got_text == "Story: unfinished continuation END",
			"local command truncated span: " .. tostring(got_text))
	`); err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionReportsWhetherGameLineWasQueued(t *testing.T) {
	tests := []struct {
		name       string
		connected  bool
		setupLua   string
		input      string
		wantSent   []string
		wantQueued bool
	}{
		{
			name:       "game command",
			connected:  true,
			input:      "look",
			wantSent:   []string{"look"},
			wantQueued: true,
		},
		{
			name:     "failed send",
			input:    "look",
			wantSent: nil,
		},
		{
			name:      "send from echo hook",
			connected: true,
			setupLua: `
				rune.hooks.on("echo", function(text)
					rune.send_raw("from-echo-hook")
					return text
				end, { priority = 1 })
			`,
			input:      "/help",
			wantSent:   []string{"from-echo-hook"},
			wantQueued: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = tt.connected
			if tt.setupLua != "" {
				if err := s.engine.DoString("setup", tt.setupLua); err != nil {
					t.Fatal(err)
				}
			}

			userInput(s, tt.input)
			if sent := net.drainSent(); !slices.Equal(sent, tt.wantSent) {
				t.Fatalf("sent = %q, want %q", sent, tt.wantSent)
			}
			if got := uiMock.drainEchoQueuedLines(); len(got) != 1 || got[0] != tt.wantQueued {
				t.Fatalf("queued-line signal = %v, want [%v]", got, tt.wantQueued)
			}
		})
	}
}

func TestDisconnectEventUpdatesStateAndNotifiesLua(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true

	s.handleNetworkOutput(network.Output{Kind: network.OutputDisconnect, ConnectionID: s.connectionID})

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

func TestSearchStateIsIndependentFromScrollState(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.clientState.ScrollMode = "live"

	s.handleUIMessage(ui.SearchStateChangedMsg(true))
	if !s.clientState.SearchActive {
		t.Fatal("search-active UI event did not update client state")
	}
	if s.clientState.ScrollMode != "live" {
		t.Fatalf("search changed scroll mode to %q", s.clientState.ScrollMode)
	}

	s.handleUIMessage(ui.SearchStateChangedMsg(false))
	if s.clientState.SearchActive {
		t.Fatal("search-close UI event left client state active")
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

	text := "  say hi;look  \r\n\t#2 north\r\n\r/quit\ntrailing  "
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
	echoed := uiMock.drainEchoed()
	if len(echoed) != len(want) {
		t.Fatalf("echoed %d physical lines, want %d: %q", len(echoed), len(want), echoed)
	}
	for _, line := range echoed {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("echo contains embedded line break: %q", line)
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
