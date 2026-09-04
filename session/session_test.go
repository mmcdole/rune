package session

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
	cleanupTestSession(t, s)
	return s, net, uiMock
}

func userInput(s *Session, text string) {
	s.handleSubmission(input.Command(text))
}

func serverData(s *Session, data string) {
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte(data)})
}

func completeLine(s *Session, line string) {
	serverData(s, line+"\r\n")
}

func serverGA(s *Session) {
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA})
}

func serverEOR(s *Session) {
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdEOR})
}

func serverBatch(s *Session, events ...network.TelnetEvent) {
	s.handleInbound(network.Inbound{
		Kind:         network.InboundBatch,
		ConnectionID: s.connectionID,
		Batch:        network.EventBatch{Events: events},
	})
}

func serverNegotiatesGMCP(s *Session) {
	serverBatch(s, network.TelnetEvent{
		Kind:    network.TelnetEventNegotiation,
		Command: network.CmdWILL,
		Option:  network.OptGMCP,
	})
}

func contains(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func assertSessionLua(t *testing.T, engine *lua.Engine, code string) {
	t.Helper()
	if err := engine.DoString("assert", code); err != nil {
		t.Fatal(err)
	}
}

// Stimulus/response flows (input->network, aliases, triggers, gags,
// local echo, slash commands) are covered end-to-end by the scenario
// suite in test/e2e/scenarios/. The tests here assert synchronous internals
// the scenario vocabulary cannot express.

func TestFragmentedLineUsesCumulativePromptUpdatesThenOneOutput(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	if err := s.engine.DoString("observe line lifecycle", `
		updates = {}
		outputs = {}
		rune.hooks.on("prompt", function(line, confirmed)
			updates[#updates + 1] = { text = line:clean(), confirmed = confirmed }
		end)
		rune.hooks.on("output", function(line)
			outputs[#outputs + 1] = line:clean()
		end)
	`); err != nil {
		t.Fatal(err)
	}

	serverData(s, "User")
	serverData(s, "name:")
	serverData(s, " accepted\r\n")

	assertSessionLua(t, s.engine, `
		assert(#updates == 2, "updates: " .. #updates)
		assert(updates[1].text == "User" and updates[1].confirmed == false)
		assert(updates[2].text == "Username:" and updates[2].confirmed == false)
		assert(#outputs == 1 and outputs[1] == "Username: accepted")
	`)
	if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"Username: accepted"}) {
		t.Fatalf("printed = %q, want completed line once", printed)
	}
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("completed line left current state: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
}

func TestSubmissionCommitsLatestPartialLineBeforeLocalOutput(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	serverData(s, "What is your ")
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "What is your " {
		t.Fatalf("first prompt update = %q", prompts)
	}

	serverData(s, "name:")
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "What is your name:" {
		t.Fatalf("expected updated prompt overlay, got %v", prompts)
	}
	uiMock.drainDisplayEvents()
	userInput(s, "/help")

	events := uiMock.drainDisplayEvents()
	if len(events) == 0 || events[0] != "commit:What is your name:" {
		t.Fatalf("first submission display event = %q, want prompt commit", events)
	}
	plainEvents := make([]string, len(events))
	for i, event := range events {
		plainEvents[i] = runetext.StripANSI(event)
	}
	if !contains(plainEvents, "echo:> /help") || !contains(plainEvents, "print:[Commands]") {
		t.Fatalf("submission output missing after commit: %q", events)
	}
	if sent := net.drainSent(); len(sent) != 0 {
		t.Fatalf("local /help sent game data: %q", sent)
	}
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("submission left partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}

	completeLine(s, "Welcome")
	if printed := uiMock.drainPrinted(); !contains(printed, "Welcome") || contains(printed, "What is your name:Welcome") {
		t.Fatalf("post-submission output reused committed prefix: %q", printed)
	}
}

func TestEverySubmissionFinishesPartialLine(t *testing.T) {
	tests := []struct {
		name      string
		connected bool
		localEcho bool
		setupLua  string
		input     string
		wantError bool
	}{
		{
			name:      "echo disabled",
			connected: true,
			input:     "/help",
		},
		{
			name:      "input hook consumes submission",
			connected: true,
			localEcho: true,
			setupLua: `
				rune.hooks.on("input", function()
					return false
				end, { priority = 1 })
			`,
			input: "consumed",
		},
		{
			name:      "eventual game send fails",
			localEcho: true,
			input:     "look",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = tt.connected
			if !tt.localEcho {
				serverBatch(s, network.TelnetEvent{
					Kind:    network.TelnetEventNegotiation,
					Command: network.CmdWILL,
					Option:  network.OptEcho,
				})
			}
			if tt.setupLua != "" {
				if err := s.engine.DoString("consume input", tt.setupLua); err != nil {
					t.Fatal(err)
				}
			}

			serverData(s, "Question:")
			uiMock.drainPrinted()
			uiMock.drainPrompts()
			userInput(s, tt.input)

			if s.prompt.active || s.partialLine.peek() != "" {
				t.Fatalf("submission left partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
			}
			if printed := uiMock.drainPrinted(); !contains(printed, "Question:") {
				t.Fatalf("submission did not commit partial line: %q", printed)
			} else if tt.wantError && !contains(printed, "not connected") {
				t.Fatalf("failed game send did not report error: %q", printed)
			}
			if !tt.localEcho && len(uiMock.drainEchoed()) != 0 {
				t.Fatal("echo-disabled submission was displayed")
			}
		})
	}
}

func TestGAAndEORConfirmAndConsumeCurrentPrompt(t *testing.T) {
	for _, tt := range []struct {
		name     string
		boundary func(*Session)
	}{
		{"GA", serverGA},
		{"EOR", serverEOR},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, _, uiMock := newTestSession(t)
			if err := s.engine.DoString("observe confirmation", `
				flags = {}
				rune.hooks.on("prompt", function(line, confirmed)
					flags[#flags + 1] = confirmed
				end)
			`); err != nil {
				t.Fatal(err)
			}

			serverData(s, "HP:100>")
			tt.boundary(s)
			assertSessionLua(t, s.engine, `
				assert(#flags == 2, "flags: " .. #flags)
				assert(flags[1] == false and flags[2] == true)
			`)
			if s.partialLine.peek() != "" || !s.prompt.active || !s.prompt.confirmed {
				t.Fatalf("confirmed prompt state = prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
			}

			uiMock.drainPrinted()
			completeLine(s, "A bell rings.")
			if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"HP:100>", "A bell rings."}) {
				t.Fatalf("printed = %q", printed)
			}
		})
	}
}

func TestPromptBoundaryInDataBatchOnlyEmitsConfirmedPrompt(t *testing.T) {
	for _, boundary := range []struct {
		name    string
		command byte
	}{
		{name: "GA", command: network.CmdGA},
		{name: "EOR", command: network.CmdEOR},
	} {
		t.Run(boundary.name, func(t *testing.T) {
			s, _, _ := newTestSession(t)
			if err := s.engine.DoString("observe batch prompt", `
				seen = {}
				rune.hooks.on("prompt", function(line, confirmed)
					seen[#seen + 1] = { text = line:clean(), confirmed = confirmed }
				end)
			`); err != nil {
				t.Fatal(err)
			}

			serverBatch(s,
				network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("HP:100>")},
				network.TelnetEvent{Kind: network.TelnetEventIAC, Command: boundary.command},
			)

			assertSessionLua(t, s.engine, `
				assert(#seen == 1, "callbacks: " .. #seen)
				assert(seen[1].text == "HP:100>")
				assert(seen[1].confirmed == true)
			`)
		})
	}
}

func TestPromptBoundariesPreservePromptAndTailOrderWithinBatch(t *testing.T) {
	for _, tt := range []struct {
		name           string
		events         []network.TelnetEvent
		wantFlags      string
		wantPrompt     string
		wantBufferTail string
		confirmed      bool
	}{
		{
			name: "data GA data",
			events: []network.TelnetEvent{
				{Kind: network.TelnetEventDataReceive, Data: []byte("First>")},
				{Kind: network.TelnetEventIAC, Command: network.CmdGA},
				{Kind: network.TelnetEventDataReceive, Data: []byte("Second>")},
			},
			wantFlags:      "true, false",
			wantPrompt:     "Second>",
			wantBufferTail: "Second>",
		},
		{
			name: "data GA data GA",
			events: []network.TelnetEvent{
				{Kind: network.TelnetEventDataReceive, Data: []byte("First>")},
				{Kind: network.TelnetEventIAC, Command: network.CmdGA},
				{Kind: network.TelnetEventDataReceive, Data: []byte("Second>")},
				{Kind: network.TelnetEventIAC, Command: network.CmdGA},
			},
			wantFlags:  "true, true",
			wantPrompt: "Second>",
			confirmed:  true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, _, uiMock := newTestSession(t)
			if err := s.engine.DoString("observe prompt records", `
				records = {}
				rune.hooks.on("prompt", function(line, confirmed)
					records[#records + 1] = {
						text = line:clean(),
						confirmed = confirmed,
					}
				end)
			`); err != nil {
				t.Fatal(err)
			}

			serverBatch(s, tt.events...)

			assertSessionLua(t, s.engine, `
				assert(#records == 2, "records: " .. #records)
				assert(records[1].text == "First>" and records[1].confirmed == true)
				assert(records[2].text == "Second>")
				local flags = tostring(records[1].confirmed) .. ", " ..
					tostring(records[2].confirmed)
				assert(flags == "`+tt.wantFlags+`", "flags: " .. flags)
			`)
			if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"First>"}) {
				t.Fatalf("printed = %q, want first confirmed prompt committed once", printed)
			}
			if !s.prompt.active || s.prompt.text != tt.wantPrompt || s.prompt.confirmed != tt.confirmed {
				t.Fatalf("current prompt = %+v, want text %q confirmed=%v", s.prompt, tt.wantPrompt, tt.confirmed)
			}
			if got := s.partialLine.peek(); got != tt.wantBufferTail {
				t.Fatalf("server-line tail = %q, want %q", got, tt.wantBufferTail)
			}
		})
	}
}

func TestOncePromptTriggerUsesFirstMatchingObservation(t *testing.T) {
	for _, tt := range []struct {
		name          string
		deliver       func(*Session)
		wantConfirmed bool
	}{
		{
			name: "same batch is confirmed",
			deliver: func(s *Session) {
				serverBatch(s,
					network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("Username:")},
					network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA},
				)
			},
			wantConfirmed: true,
		},
		{
			name: "later boundary follows partial match",
			deliver: func(s *Session) {
				serverData(s, "Username:")
				serverGA(s)
			},
			wantConfirmed: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := newTestSession(t)
			if err := s.engine.DoString("once prompt trigger", `
				trigger_flags = {}
				hook_flags = {}
				rune.trigger.exact("Username:", function(_, ctx)
					trigger_flags[#trigger_flags + 1] = ctx.confirmed
				end, { on = "prompt", once = true })
				rune.hooks.on("prompt", function(_, confirmed)
					hook_flags[#hook_flags + 1] = confirmed
				end, { priority = 200 })
			`); err != nil {
				t.Fatal(err)
			}

			tt.deliver(s)
			want := "false"
			if tt.wantConfirmed {
				want = "true"
			}
			assertSessionLua(t, s.engine, `
				assert(#trigger_flags == 1, "once trigger fires: " .. #trigger_flags)
				assert(trigger_flags[1] == `+want+`, "unexpected first observation")
				assert(#rune.trigger.list() == 0, "once trigger remained registered")
				assert(hook_flags[#hook_flags] == true, "independent hook missed confirmation")
			`)
		})
	}
}

func TestInputRewriteControlsEchoHistoryAndDispatch(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("rewrite input", `
		history_seen_by_input = -1
		history_seen_by_echo = -1
		rune.hooks.on("input", function(text)
			history_seen_by_input = #rune.history.get()
			assert(text == "north")
			return "east"
		end, { name = "test-rewrite", priority = 90 })
		rune.hooks.on("echo", function(text)
			history_seen_by_echo = #rune.history.get()
		end, { name = "test-history-before-echo", priority = 90 })
	`); err != nil {
		t.Fatal(err)
	}

	s.handleUIEvent(ui.InputSubmittedMsg{
		Submission: input.Command("north"),
		NextDraft:  "north",
	})

	if got := s.GetInput(); got != "north" {
		t.Fatalf("retained editor draft = %q, want authored text", got)
	}
	if got := net.drainSent(); !slices.Equal(got, []string{"east"}) {
		t.Fatalf("wire submission = %q, want rewritten text", got)
	}
	if got := s.GetHistoryEntries(); !slices.Equal(got, []input.Submission{input.Command("east")}) {
		t.Fatalf("history = %+v, want rewritten submission", got)
	}
	echoed := uiMock.drainEchoed()
	if len(echoed) != 1 || runetext.StripANSI(echoed[0]) != "> east" {
		t.Fatalf("echo = %q, want rewritten submission", echoed)
	}
	assertSessionLua(t, s.engine, `
		assert(history_seen_by_input == 0)
		assert(history_seen_by_echo == 1)
	`)
}

func TestConsumedInputHasNoEchoHistoryOrDispatch(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("consume input", `
		rune.hooks.on("input", function()
			rune.echo("input rejected")
			return false
		end, { name = "test-consume", priority = 90 })
	`); err != nil {
		t.Fatal(err)
	}

	s.handleSubmission(input.Command("north"))

	if sent := net.drainSent(); len(sent) != 0 {
		t.Fatalf("consumed input sent %q", sent)
	}
	if history := s.GetHistoryEntries(); len(history) != 0 {
		t.Fatalf("consumed input entered history: %+v", history)
	}
	if echoed := uiMock.drainEchoed(); len(echoed) != 0 {
		t.Fatalf("consumed input was locally echoed: %q", echoed)
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "input rejected") {
		t.Fatalf("consuming handler feedback missing: %q", printed)
	}
}

func TestStructuredCommandRewriteHasNoEchoHistoryOrDispatch(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("rewrite command as structured text", `
		rune.hooks.on("input", function()
			return "east\nwest"
		end, { name = "test-structured-rewrite", priority = 90 })
	`); err != nil {
		t.Fatal(err)
	}

	s.handleSubmission(input.Command("north"))

	if sent := net.drainSent(); len(sent) != 0 {
		t.Fatalf("structured command rewrite sent %q", sent)
	}
	if history := s.GetHistoryEntries(); len(history) != 0 {
		t.Fatalf("structured command rewrite entered history: %+v", history)
	}
	if echoed := uiMock.drainEchoed(); len(echoed) != 0 {
		t.Fatalf("structured command rewrite was locally echoed: %q", echoed)
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "command rewrite must be valid text on one line") {
		t.Fatalf("structured command rewrite produced no useful error: %q", printed)
	}
}

func TestConfirmedPromptBatchFinishesAfterRewriteOrGag(t *testing.T) {
	tests := []struct {
		name        string
		setup       string
		prompt      string
		wantSend    string
		wantCommit  string
		wantPrinted []string
	}{
		{
			name: "rewrite",
			setup: `
				rune.trigger.exact("Username:", "player", { on = "prompt" })
				rune.hooks.on("prompt", function(line, confirmed)
					assert(confirmed == true)
					return "Final login prompt"
				end, { priority = 200 })
			`,
			prompt:      "Username:",
			wantSend:    "player",
			wantCommit:  "commit:Final login prompt",
			wantPrinted: []string{"Final login prompt"},
		},
		{
			name: "gag",
			setup: `
				rune.trigger.exact("Password:", "secret", {
					on = "prompt",
					gag = true,
				})
			`,
			prompt:     "Password:",
			wantSend:   "secret",
			wantCommit: "commit:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = true
			if err := s.engine.DoString("confirmed prompt action", tt.setup); err != nil {
				t.Fatal(err)
			}
			uiMock.drainDisplayEvents()

			serverBatch(s,
				network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte(tt.prompt)},
				network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA},
			)

			if sent := net.drainSent(); !slices.Equal(sent, []string{tt.wantSend}) {
				t.Fatalf("sent = %q", sent)
			}
			if printed := uiMock.drainPrinted(); !slices.Equal(printed, tt.wantPrinted) {
				t.Fatalf("printed = %q, want %q", printed, tt.wantPrinted)
			}
			commits := 0
			for _, event := range uiMock.drainDisplayEvents() {
				if event == tt.wantCommit {
					commits++
				}
			}
			if commits != 1 {
				t.Fatalf("final prompt committed %d times, want once", commits)
			}
			if s.prompt.active || s.partialLine.peek() != "" {
				t.Fatalf("finished prompt remained active: prompt=%+v line=%q", s.prompt, s.partialLine.peek())
			}
		})
	}
}

func TestTrailingBareCRIsImmediateOutputNotPrompt(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	uiMock.drainPrompts()
	if err := s.engine.DoString("observe trailing bare CR", `
		prompts = 0
		outputs = {}
		rune.hooks.on("prompt", function() prompts = prompts + 1 end)
		rune.hooks.on("output", function(line) outputs[#outputs + 1] = line:clean() end)
	`); err != nil {
		t.Fatal(err)
	}

	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("complete\r")})
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("\nnext\r")})

	assertSessionLua(t, s.engine, `
		assert(prompts == 0, "prompt callbacks: " .. prompts)
		assert(#outputs == 2 and outputs[1] == "complete" and outputs[2] == "next")
	`)
	if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"complete", "next"}) {
		t.Fatalf("printed = %q", printed)
	}
	if prompts := uiMock.drainPrompts(); len(prompts) != 0 {
		t.Fatalf("ordinary lines produced prompt updates: %q", prompts)
	}
}

func TestBareCRLinePrecedesLaterProtocolEffectsInBatch(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("observe CR and GMCP order", `
		events = {}
		rune.hooks.on("output", function(line)
			if line:clean() == "complete" then events[#events + 1] = "output" end
		end)
		rune.hooks.on("gmcp_enabled", function()
			events[#events + 1] = "gmcp"
		end, { priority = 1 })
	`); err != nil {
		t.Fatal(err)
	}

	serverBatch(s,
		network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("complete\r")},
		network.TelnetEvent{Kind: network.TelnetEventDataSend, Data: []byte{network.CmdIAC, network.CmdDO, network.OptGMCP}},
		network.TelnetEvent{Kind: network.TelnetEventNegotiation, Command: network.CmdWILL, Option: network.OptGMCP},
	)

	assertSessionLua(t, s.engine, `
		assert(#events == 2, "events: " .. table.concat(events, ","))
		assert(events[1] == "output" and events[2] == "gmcp",
			"wire order: " .. table.concat(events, ","))
	`)
}

func TestEmptyPromptBoundaryDoesNothing(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	serverGA(s)
	serverEOR(s)
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("empty boundaries changed partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("empty boundaries printed %q", printed)
	}
}

func TestCRBeforePromptBoundaryIsAlwaysAnOrdinaryLine(t *testing.T) {
	for _, tt := range []struct {
		name    string
		deliver func(*Session)
	}{
		{
			name: "same batch",
			deliver: func(s *Session) {
				serverBatch(s,
					network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("complete\r")},
					network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA},
				)
			},
		},
		{
			name: "separate batches",
			deliver: func(s *Session) {
				serverData(s, "complete\r")
				serverGA(s)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, _, uiMock := newTestSession(t)
			if err := s.engine.DoString("observe CR classification", `
				prompt_count = 0
				rune.hooks.on("prompt", function() prompt_count = prompt_count + 1 end)
			`); err != nil {
				t.Fatal(err)
			}

			tt.deliver(s)
			serverData(s, "\nnext\r\n")
			if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"complete", "next"}) {
				t.Fatalf("printed = %q, want complete and next", printed)
			}
			assertSessionLua(t, s.engine, `assert(prompt_count == 0)`)
		})
	}
}

func TestSuccessfulSendFinishesPartialLineAndFlushesSpan(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("open span", `
		fired = 0
		rune.trigger.starts("Story:", function()
			fired = fired + 1
		end, { span = { to = "NEVER", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	completeLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverData(s, "Tundra tells you: meet me at the")
	uiMock.drainPrompts()

	if err := s.Send("look"); err != nil {
		t.Fatal(err)
	}

	if printed := uiMock.drainPrinted(); len(printed) != 1 || printed[0] != "Tundra tells you: meet me at the" {
		t.Fatalf("send did not commit partial line: %q", printed)
	}
	if prompts := uiMock.drainPrompts(); len(prompts) != 1 || prompts[0] != "" {
		t.Fatalf("send did not clear overlay: %q", prompts)
	}
	if err := s.engine.DoString("assert", `assert(fired == 1, "send did not flush span")`); err != nil {
		t.Fatal(err)
	}
}

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

	serverData(s, "Username:")

	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "player" {
		t.Fatalf("prompt action sent %q, want player", sent)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 1 || printed[0] != "Final login prompt" {
		t.Fatalf("send committed %q, want final prompt rewrite", printed)
	}
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("trigger send left partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
}

func TestMultipleSendsDuringPromptUpdateFinishOnce(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("multiple prompt sends", `
		rune.hooks.on("prompt", function(line)
			if line:clean() == "Choose:" then
				rune.send("one")
				rune.send("two")
				return "Final choice:"
			end
		end)
	`); err != nil {
		t.Fatal(err)
	}

	serverData(s, "Choose:")
	if sent := net.drainSent(); !slices.Equal(sent, []string{"one", "two"}) {
		t.Fatalf("sent = %q", sent)
	}
	if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"Final choice:"}) {
		t.Fatalf("committed prompts = %q, want one final prompt", printed)
	}
}

func TestSendFromConfirmedPromptSpanFlushCommitsOnce(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("span sends before confirmed prompt hook", `
		rune.trigger.starts("Story:", function()
			rune.send("save")
		end, { span = { to = "NEVER", max = 8 } })
		rune.hooks.on("prompt", function(line, confirmed)
			if confirmed then return "Final HP>" end
		end, { priority = 200 })
	`); err != nil {
		t.Fatal(err)
	}

	completeLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverData(s, "HP>")
	uiMock.drainPrinted()
	serverGA(s)

	if sent := net.drainSent(); !slices.Equal(sent, []string{"save"}) {
		t.Fatalf("span action sent = %q", sent)
	}
	if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"Final HP>"}) {
		t.Fatalf("confirmed prompt commits = %q, want one final rewrite", printed)
	}
}

func TestConfirmedPromptSpanConnectionChangeStopsPromptDispatch(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("disconnect while closing prompt span", `
		prompt_hooks = 0
		rune.trigger.starts("Story:", function()
			rune.disconnect()
		end, { span = { to = "NEVER", max = 8 } })
		rune.hooks.on("prompt", function()
			prompt_hooks = prompt_hooks + 1
		end, { priority = 200 })
	`); err != nil {
		t.Fatal(err)
	}

	completeLine(s, "Story: unfinished")
	serverBatch(s,
		network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("HP>")},
		network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA},
	)

	if net.connected {
		t.Fatal("span action did not disconnect")
	}
	assertSessionLua(t, s.engine, `
		assert(prompt_hooks == 0,
			"old prompt hooks ran after the span changed connections")
	`)
}

func TestOutputTriggerSendProcessesAndCommitsLaterTailInSameData(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("send from output then rewrite prompt", `
		rune.trigger.exact("Fire", "look")
		rune.hooks.on("prompt", function(line)
			if line:clean() == "HP>" then return "Final HP>" end
		end, { priority = 200 })
	`); err != nil {
		t.Fatal(err)
	}

	serverData(s, "Fire\r\nHP>")
	if sent := net.drainSent(); !slices.Equal(sent, []string{"look"}) {
		t.Fatalf("sent = %q", sent)
	}
	if printed := uiMock.drainPrinted(); !slices.Equal(printed, []string{"Fire", "Final HP>"}) {
		t.Fatalf("printed = %q, want line then final prompt", printed)
	}
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("deferred send left partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
}

func TestFailedSendLeavesPartialLineOpen(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	serverData(s, "User")
	uiMock.drainPrompts()
	if err := s.Send("player"); err == nil {
		t.Fatal("send while disconnected succeeded")
	}
	serverData(s, "name:")
	if prompts := uiMock.drainPrompts(); !slices.Equal(prompts, []string{"Username:"}) {
		t.Fatalf("partial line after failed send = %q", prompts)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("failed send committed partial line: %q", printed)
	}
}

func TestGMCPSendDoesNotFinishPartialLine(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	serverNegotiatesGMCP(s)
	net.drainGMCPSent()
	serverData(s, "User")
	uiMock.drainPrompts()

	if err := s.GMCPSend("Core.Hello", `{}`); err != nil {
		t.Fatal(err)
	}
	serverData(s, "name:")

	if prompts := uiMock.drainPrompts(); !slices.Equal(prompts, []string{"Username:"}) {
		t.Fatalf("GMCP send changed partial line: %q", prompts)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("GMCP send committed partial line: %q", printed)
	}
}

func TestSuccessfulSendWithoutPartialLineDoesNotFlushOpenSpan(t *testing.T) {
	s, net, _ := newTestSession(t)
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

	completeLine(s, "Story: unfinished")
	if err := s.Send("look"); err != nil {
		t.Fatal(err)
	}
	if err := s.engine.DoString("assert", `assert(fired == 0, "unrelated send flushed span")`); err != nil {
		t.Fatal(err)
	}

	completeLine(s, "continuation END")
	if err := s.engine.DoString("assert", `
		assert(fired == 1, "span did not finish")
		assert(got_text == "Story: unfinished continuation END",
			"span was truncated: " .. tostring(got_text))
	`); err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionClosesSpanWithoutPrintingGaggedPartialLine(t *testing.T) {
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

	completeLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverData(s, "Password:")
	if prompts := uiMock.drainPrompts(); len(prompts) == 0 || prompts[len(prompts)-1] != "" {
		t.Fatalf("gagged partial should leave an empty overlay, got %q", prompts)
	}

	userInput(s, "/help")
	if err := s.engine.DoString("assert", `assert(fired == 1, "submission did not flush span")`); err != nil {
		t.Fatal(err)
	}
	if contains(uiMock.drainPrinted(), "Password:") {
		t.Fatal("gagged partial was committed to scrollback")
	}
}

func TestLocalSubmissionCommitsPartialLineAndFlushesSpan(t *testing.T) {
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

	completeLine(s, "Story: unfinished")
	uiMock.drainPrinted()
	serverData(s, "Username:")
	uiMock.drainPrompts()

	userInput(s, "/help")
	if sent := net.drainSent(); len(sent) != 0 {
		t.Fatalf("local command unexpectedly reached network: %q", sent)
	}
	if !contains(uiMock.drainPrinted(), "Username:") {
		t.Fatal("local command did not commit partial line")
	}
	if err := s.engine.DoString("assert", `assert(fired == 1, "local command did not flush span")`); err != nil {
		t.Fatal(err)
	}

	completeLine(s, "continuation END")
	if err := s.engine.DoString("assert", `
		assert(fired == 1, "old span fired twice")
	`); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionChangeDiscardsPartialLineAndRemainingData(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("disconnect on first line", `
		rune.hooks.on("output", function(line)
			if line:clean() == "old one" then rune.disconnect() end
		end, { priority = 1 })
	`); err != nil {
		t.Fatal(err)
	}

	serverData(s, "old one\r\nold two\r\nold tail")
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("old connection data survived: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
	if contains(uiMock.drainPrinted(), "old two") {
		t.Fatal("processed data following a connection-changing hook")
	}
	completeLine(s, "new output")
	if printed := uiMock.drainPrinted(); !contains(printed, "new output") {
		t.Fatalf("new output missing: %q", printed)
	}
}

func TestConnectionChangeLaterInBatchCancelsDeferredSendFinish(t *testing.T) {
	for _, tt := range []struct {
		name          string
		change        string
		finishConnect bool
	}{
		{
			name:   "disconnect",
			change: `rune.disconnect()`,
		},
		{
			name:          "reconnect",
			change:        `rune.connect("next.example:4000")`,
			finishConnect: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = true
			serverNegotiatesGMCP(s)
			net.drainGMCPSent()
			if err := s.engine.DoString("change connection after prompt send", `
				rune.hooks.on("prompt", function(line, confirmed)
					if line:clean() == "Old prompt>" then
						assert(confirmed == true)
						rune.send("answer")
						return "Old prompt rewritten>"
					end
				end)
				rune.gmcp.on("Core.Switch", function()
					`+tt.change+`
				end)
			`); err != nil {
				t.Fatal(err)
			}
			oldConnection := s.connectionID
			uiMock.drainPrinted()
			uiMock.drainDisplayEvents()

			serverBatch(s,
				network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("Old prompt>")},
				network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA},
				network.TelnetEvent{Kind: network.TelnetEventSubnegotiation, Option: network.OptGMCP, Data: []byte("Core.Switch {}")},
				network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("stale data\r\n")},
			)
			if tt.finishConnect {
				awaitInternalEvent(t, s)
			}

			if sent := net.drainSent(); !slices.Equal(sent, []string{"answer"}) {
				t.Fatalf("accepted sends = %q, want answer", sent)
			}
			if s.connectionID == oldConnection {
				t.Fatal("connection-changing callback did not advance the generation")
			}
			if s.activeBatch != nil || s.prompt.active || s.partialLine.peek() != "" {
				t.Fatalf("old inbound state survived: batch=%+v prompt=%+v tail=%q",
					s.activeBatch, s.prompt, s.partialLine.peek())
			}
			for _, event := range uiMock.drainDisplayEvents() {
				if event == "commit:Old prompt rewritten>" || strings.Contains(event, "stale data") {
					t.Fatalf("old batch escaped after connection change: %q", event)
				}
			}

			serverData(s, "New prompt>")
			if !s.prompt.active || s.prompt.text != "New prompt>" || s.partialLine.peek() != "New prompt>" {
				t.Fatalf("new connection tail restored old state: prompt=%+v tail=%q",
					s.prompt, s.partialLine.peek())
			}
			if contains(uiMock.drainPrinted(), "Old prompt rewritten>") {
				t.Fatal("deferred finish committed the old prompt after connection change")
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

	serverData(s, "previous Username:")
	if net.connected {
		t.Fatal("prompt hook did not disconnect")
	}
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("outer handler restored old partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
	for _, prompt := range uiMock.drainPrompts() {
		if prompt != "" {
			t.Fatalf("previous prompt overlay was repainted after disconnect: %q", prompt)
		}
	}
}

func TestStaleInboundBatchCannotChangeCurrentConnection(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	oldConnection := s.connectionID
	s.Disconnect()
	uiMock.drainPrinted()
	uiMock.drainPrompts()
	serverData(s, "new Username:")
	uiMock.drainPrompts()

	s.handleInbound(network.Inbound{Kind: network.InboundBatch, ConnectionID: oldConnection, Batch: network.EventBatch{Events: []network.TelnetEvent{
		{Kind: network.TelnetEventDataReceive, Data: []byte("old")},
		{Kind: network.TelnetEventIAC, Command: network.CmdGA},
	}}})
	if got := s.partialLine.peek(); got != "new Username:" {
		t.Fatalf("stale facts changed partial line to %q", got)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("stale facts printed %q", printed)
	}
}

func TestRequiredProtocolReplyFailureDisconnectsAndStopsBatch(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	s.clientState.Connected = true
	connectionID := s.connectionID
	// The mock is deliberately not connected, so queuing the required reply
	// fails before the later server-data effect can run.
	s.handleInbound(network.Inbound{
		Kind:         network.InboundBatch,
		ConnectionID: connectionID,
		Batch: network.EventBatch{Events: []network.TelnetEvent{
			{Kind: network.TelnetEventDataSend, Data: []byte{network.CmdIAC, network.CmdDO, network.OptEcho}},
			{Kind: network.TelnetEventDataReceive, Data: []byte("must not print\r\n")},
		}},
	})

	if s.connectionID == connectionID || s.clientState.Connected {
		t.Fatalf("failed required reply did not disconnect: id=%d state=%+v", s.connectionID, s.clientState)
	}
	if contains(uiMock.drainPrinted(), "must not print") {
		t.Fatal("batch continued after required protocol reply failed")
	}
	if net.connected {
		t.Fatal("network remained connected")
	}
}

func TestInboundDisconnectUpdatesStateAndRunsHook(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true

	s.handleInbound(network.Inbound{Kind: network.InboundDisconnect, ConnectionID: s.connectionID})

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

	if err := s.engine.DoString("request reload", `rune.reload()`); err != nil {
		t.Fatal(err)
	}

	// Reload is queued, not executed inline.
	select {
	case event := <-s.internalEvents:
		s.handleInternalEvent(event)
	default:
		t.Fatal("reload did not queue an internal event")
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

func TestWindowSizeChangeDispatchesHookWithStateInSync(t *testing.T) {
	s, _, _ := newTestSession(t)

	assertSessionLua(t, s.engine, `
		captured = nil
		rune.hooks.on("window_size_changed", function(w, h)
			captured = {
				w = w, h = h,
				w_type = type(w), h_type = type(h),
				state_w = rune.state.width, state_h = rune.state.height,
			}
		end)
	`)

	// The first reported size and later resizes share this path.
	s.handleUIEvent(ui.WindowSizeChangedMsg{Width: 120, Height: 40})

	assertSessionLua(t, s.engine, `
		assert(captured, "window_size_changed did not fire")
		assert(captured.w_type == "number" and captured.h_type == "number",
			"args must be numbers")
		assert(captured.w == 120 and captured.h == 40,
			"args " .. tostring(captured.w) .. "x" .. tostring(captured.h))
		assert(captured.state_w == 120 and captured.state_h == 40,
			"rune.state must already hold the new size during the callback")
	`)
}

func TestResizeHookLayoutChangeAppliesInSameCycle(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	assertSessionLua(t, s.engine, `
		rune.hooks.on("window_size_changed", function(w)
			if w < 80 then
				rune.ui.layout({ type = "column", children = {
					{ type = "pane", name = "output", border = "none" },
					{ type = "input" },
				} })
			end
		end)
	`)
	uiMock.drainLayoutPushes()

	s.handleUIEvent(ui.WindowSizeChangedMsg{Width: 60, Height: 40})

	if uiMock.drainLayoutPushes() == 0 {
		t.Error("layout change from a resize handler was not pushed during the resize cycle")
	}
	want := ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{
				Type: ui.LayoutTypePane, Name: ui.OutputPaneName,
				Border: ui.PaneBorderNone,
				Size:   ui.Fraction(1),
			},
			{
				Type: ui.LayoutTypeInput,
				Size: ui.AutoSize(),
			},
		},
	}}
	if got := uiMock.pushedLayout(); !reflect.DeepEqual(got, want) {
		t.Fatalf("pushed layout = %#v, want %#v", got, want)
	}
}

func TestBarRefreshPublishesOnlySuccessfulSnapshots(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	uiMock.drainBarPushes()

	assertSessionLua(t, s.engine, `rune.bars.clear()`)
	s.pushBarUpdates()

	count, bars := uiMock.drainBarPushes()
	if count != 1 {
		t.Fatalf("UpdateBars calls = %d, want one empty snapshot", count)
	}
	if len(bars) != 0 {
		t.Fatalf("UpdateBars payload = %#v, want no active bars", bars)
	}

	assertSessionLua(t, s.engine, `
		rune.bars._render_all = function()
			error("transient render failure")
		end
	`)
	uiMock.drainBarPushes()
	s.pushBarUpdates()
	if count, _ := uiMock.drainBarPushes(); count != 0 {
		t.Fatalf("failed bar render published %d snapshots, want none", count)
	}
}

func TestReloadRestoresDimensionsWithoutSyntheticResize(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.handleUIEvent(ui.WindowSizeChangedMsg{Width: 100, Height: 30})

	initLua := `
		width_at_load = rune.state.width
		height_at_load = rune.state.height
		resize_fired = false
		rune.hooks.on("window_size_changed", function() resize_fired = true end)
	`
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	if err := os.WriteFile(initPath, []byte(initLua), 0o644); err != nil {
		t.Fatal(err)
	}

	s.handleReloadRequested()

	assertSessionLua(t, s.engine, `
		assert(width_at_load == 100 and height_at_load == 30,
			"init.lua must see the restored size, got " ..
			tostring(width_at_load) .. "x" .. tostring(height_at_load))
		assert(rune.state.width == 100 and rune.state.height == 30)
		assert(resize_fired == false, "reload must not fire a synthetic resize")
	`)
}

func TestConfigSetPublishesRuntimeChangesAndOneFinalReloadSnapshot(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	uiMock.drainConfigPushes()

	if uiMock.pushedConfig().KeepInput {
		t.Fatal("keep_input must default off")
	}
	if uiMock.pushedConfig().Numpad {
		t.Fatal("numpad must default off")
	}
	if uiMock.pushedConfig().Mouse {
		t.Fatal("mouse must default off")
	}

	assertSessionLua(t, s.engine, `rune.config.set("keep_input", true)`)
	if !uiMock.pushedConfig().KeepInput {
		t.Fatal("keep_input=true did not reach the UI")
	}
	assertSessionLua(t, s.engine, `assert(rune.config.get("keep_input") == true)`)
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].KeepInput {
		t.Fatalf("runtime config pushes = %+v, want one keep_input=true", pushes)
	}

	assertSessionLua(t, s.engine, `rune.config.set("numpad", true)`)
	if !uiMock.pushedConfig().Numpad {
		t.Fatal("numpad=true did not reach the UI")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].Numpad {
		t.Fatalf("runtime config pushes = %+v, want one numpad=true", pushes)
	}

	assertSessionLua(t, s.engine, `rune.config.set("mouse", true)`)
	if !uiMock.pushedConfig().Mouse {
		t.Fatal("mouse=true did not reach the UI")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].Mouse {
		t.Fatalf("runtime config pushes = %+v, want one mouse=true", pushes)
	}

	// Parser settings use the same config publication path. Each update must
	// retain the current UI-facing values.
	assertSessionLua(t, s.engine, `
		rune.config.set("command_separator", "|")
		rune.config.set("history_character", "^")
	`)
	pushes := uiMock.drainConfigPushes()
	if len(pushes) != 2 {
		t.Fatalf("parser config pushes = %+v, want exactly two snapshots", pushes)
	}
	for i, push := range pushes {
		if !push.KeepInput || !push.Numpad || !push.Mouse {
			t.Fatalf("parser config push %d reset UI config: %+v", i, push)
		}
	}

	// Reload without an init.lua reverts to defaults.
	s.handleReloadRequested()
	if uiMock.pushedConfig().KeepInput {
		t.Fatal("reload did not reset keep_input to its default")
	}
	if uiMock.pushedConfig().Numpad {
		t.Fatal("reload did not reset numpad to its default")
	}
	if uiMock.pushedConfig().Mouse {
		t.Fatal("reload did not reset mouse to its default")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || pushes[0].KeepInput || pushes[0].Numpad || pushes[0].Mouse {
		t.Fatalf("default reload pushes = %+v, want exactly one final false snapshot", pushes)
	}
	assertSessionLua(t, s.engine, `
		assert(rune.config.get("command_separator") == ";")
		assert(rune.config.get("history_character") == "!")
	`)

	// Reload with an init.lua reapplies one final snapshot of all configured values.
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	initLua := `
rune.config.set("command_separator", "||")
rune.config.set("history_character", "?")
rune.config.set("keep_input", true)
rune.config.set("numpad", true)
rune.config.set("mouse", true)
`
	if err := os.WriteFile(initPath, []byte(initLua), 0o644); err != nil {
		t.Fatal(err)
	}
	s.handleReloadRequested()
	if !uiMock.pushedConfig().KeepInput {
		t.Fatal("reload did not reapply keep_input from init.lua")
	}
	if !uiMock.pushedConfig().Numpad {
		t.Fatal("reload did not reapply numpad from init.lua")
	}
	if !uiMock.pushedConfig().Mouse {
		t.Fatal("reload did not reapply mouse from init.lua")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].KeepInput || !pushes[0].Numpad || !pushes[0].Mouse {
		t.Fatalf("configured reload pushes = %+v, want exactly one final true snapshot", pushes)
	}
	assertSessionLua(t, s.engine, `
		assert(rune.config.get("command_separator") == "||")
		assert(rune.config.get("history_character") == "?")
		assert(rune.config.get("keep_input") == true)
		assert(rune.config.get("numpad") == true)
		assert(rune.config.get("mouse") == true)
	`)
}

func TestHistoryDedupAndTrim(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.historyLimit = 3

	for _, cmd := range []string{"a", "a", "b", "", "c", "d"} {
		s.AddToHistory(cmd)
	}
	got := s.GetHistoryEntries()
	want := []input.Submission{input.Command("b"), input.Command("c"), input.Command("d")}
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

	if len(uiMock.submissions) != 1 || uiMock.submissions[0] != want {
		t.Fatalf("explicit input updates = %+v, want [%+v]", uiMock.submissions, want)
	}
	if got := s.GetInput(); got != want.Text {
		t.Fatalf("Session input mirror = %q, want %q", got, want.Text)
	}
	if got, wantCursor := s.InputGetCursor(), len(want.Text); got != wantCursor {
		t.Fatalf("Session cursor mirror = %d, want %d", got, wantCursor)
	}
}

func TestSubmissionEventAppliesNextDraftBeforeInputHooks(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("observe submission ordering", `
		observed = {}
		rune.hooks.on("input_changed", function(text)
			observed[#observed + 1] = "changed:" .. text .. ":" .. rune.input.get()
		end, { priority = 1 })
		rune.hooks.on("input", function(text)
			observed[#observed + 1] = "input:" .. text .. ":" .. rune.input.get()
		end, { priority = 1 })
	`); err != nil {
		t.Fatal(err)
	}

	s.handleUIEvent(ui.InputSubmittedMsg{
		Submission: input.Command("café"),
		NextDraft:  "café",
	})

	if got := s.GetInput(); got != "café" {
		t.Fatalf("input after accepted submission = %q, want retained draft", got)
	}
	if got := s.InputGetCursor(); got != len("café") {
		t.Fatalf("cursor after accepted submission = %d, want end", got)
	}
	assertSessionLua(t, s.engine, `
		assert(#observed == 2, "observed " .. table.concat(observed, " | "))
		assert(observed[1] == "changed:café:café", observed[1])
		assert(observed[2] == "input:café:café", observed[2])
	`)
}

func TestInputCursorConvertsAtUIBoundary(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	s.handleUIEvent(ui.InputChangedMsg{Text: "café gob", Cursor: 8})
	if got, want := s.InputGetCursor(), len("café gob"); got != want {
		t.Fatalf("cursor after input change = %d, want %d", got, want)
	}

	s.handleUIEvent(ui.CursorMovedMsg{Cursor: 4})
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

	s.handleUIEvent(ui.SearchStateChangedMsg(true))
	if !s.clientState.SearchActive {
		t.Fatal("search-active UI event did not update client state")
	}
	if s.clientState.ScrollMode != "live" {
		t.Fatalf("search changed scroll mode to %q", s.clientState.ScrollMode)
	}

	s.handleUIEvent(ui.SearchStateChangedMsg(false))
	if s.clientState.SearchActive {
		t.Fatal("search-close UI event left client state active")
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

func TestConnectingHookSendsOnTheExistingConnection(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true
	s.engine.UpdateState(s.clientState)

	if err := s.engine.DoString("hook", `
		rune.hooks.on("connecting", function() rune.send_raw("goodbye") end)
	`); err != nil {
		t.Fatal(err)
	}

	s.Connect("mud.example.com:4000")

	if sent := net.drainSent(); !slices.Equal(sent, []string{"goodbye"}) {
		t.Fatalf("connecting hook sent %q, want goodbye on the old connection", sent)
	}
	if s.clientState.Connected {
		t.Fatal("retiring the old socket left clientState.Connected true")
	}
	assertSessionLua(t, s.engine, `assert(rune.state.connected == false, "rune.state.connected still true while dialing")`)
}

func TestDisconnectingHookSendsOnTheLiveConnection(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("hook", `
		rune.hooks.on("disconnecting", function() rune.send_raw("farewell") end)
	`); err != nil {
		t.Fatal(err)
	}

	s.Disconnect()

	if sent := net.drainSent(); !slices.Equal(sent, []string{"farewell"}) {
		t.Fatalf("disconnecting hook sent %q, want farewell on the live connection", sent)
	}
}

// TestPresentationChangesCoalesceIntoOnePushPerEvent: a bind that installs a
// layout and then flips a pane several times must reach the UI as one layout
// snapshot carrying the final gate, never as a sequence of intermediate
// snapshots.
func TestPresentationChangesCoalesceIntoOnePushPerEvent(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `
		rune.bind("f9", function()
			rune.ui.layout({ type = "column", children = {
				{ type = "pane", name = "group", size = 5 },
				{ type = "pane", name = "output", border = "none" },
				{ type = "input" },
			} })
			rune.pane.hide("group")
			rune.pane.show("group")
			rune.pane.hide("group")
		end)
	`)
	// Registering the bind outside a handler leaves the flag set. Start the
	// tested event clean so the count below is the callback's own doing.
	s.flushPresentation()
	uiMock.drainLayoutPushes()
	uiMock.drainBarPushes()

	s.handleUIEvent(ui.ExecuteBindMsg("f9"))

	if n := uiMock.drainLayoutPushes(); n != 1 {
		t.Fatalf("layout pushes during one bind = %d, want exactly one", n)
	}
	if n, _ := uiMock.drainBarPushes(); n != 1 {
		t.Fatalf("bar pushes during one bind = %d, want exactly one", n)
	}
	got := uiMock.pushedLayout()
	if len(got.Root.Children) != 3 || got.Root.Children[0].Name != "group" || !got.Root.Children[0].Hidden {
		t.Fatalf("pushed layout = %#v, want the group pane hidden", got)
	}
}

// TestBootPublishesOnePresentationSnapshot locks in the boot cost: core
// scripts register many binds, and none of them may push on its own.
func TestBootPublishesOnePresentationSnapshot(t *testing.T) {
	_, _, uiMock := newTestSession(t)
	if n := uiMock.drainLayoutPushes(); n != 1 {
		t.Fatalf("layout pushes during boot = %d, want exactly one", n)
	}
}

// TestTimerCallbacksFlushPresentationOnce covers the timer lane of the
// event loop: presentation changes made by a timer callback publish once when
// the timer handler returns.
func TestTimerCallbacksFlushPresentationOnce(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `
		rune.timer.after(0.01, function()
			rune.pane.hide("output")
			rune.pane.show("output")
		end)
	`)
	s.flushPresentation()
	uiMock.drainLayoutPushes()

	select {
	case evt := <-s.timerEvents:
		s.handleTimer(evt)
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire")
	}
	if n := uiMock.drainLayoutPushes(); n != 1 {
		t.Fatalf("layout pushes from one timer callback = %d, want exactly one", n)
	}
	if hidden, found := uiMock.pushedLayout().PaneHidden(ui.OutputPaneName); !found || hidden {
		t.Fatalf("pushed layout output hidden = %v, found = %v; want false, true", hidden, found)
	}
}
