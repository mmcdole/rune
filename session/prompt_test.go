package session

import (
	"slices"
	"testing"

	"github.com/mmcdole/rune/network"
	runetext "github.com/mmcdole/rune/text"
)

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
