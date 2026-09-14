package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

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
		assert(history_seen_by_echo == 0)
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

func TestControlCommandRewriteHasNoEchoHistoryOrDispatch(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("rewrite command as structured text", `
		rune.hooks.on("input", function()
			return "east\027west"
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
	if printed := uiMock.drainPrinted(); !contains(printed, "command rewrite must be valid command text") {
		t.Fatalf("structured command rewrite produced no useful error: %q", printed)
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

func TestVerbatimSubmissionPreservesPhysicalLines(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	assertSessionLua(t, s.engine, `rune.alias.exact("aliased", "expanded")`)

	text := "  say hi;look  \r\n\t#2 north\r\n\r/quit\naliased\ntrailing  \n"
	s.handleSubmission(input.Verbatim(text))

	want := []string{"  say hi;look  ", "\t#2 north", "", "/quit", "aliased", "trailing  ", ""}
	got := net.drainSent()
	if len(got) != len(want) {
		t.Fatalf("sent %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sent[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if history := s.GetHistoryEntries(); len(history) != 1 || history[0] != input.Verbatim(strings.Join(want, "\n")) {
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

func TestCommandBatchProcessesLinesAndKeepsHistory(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	source := "/lua rune.alias.exact('batch', 'answer 42')\n\tbatch\n/lua -- comment ends on this line\nbatch;look\n\n"
	s.handleSubmission(input.Command(source))
	if got := net.drainSent(); !slices.Equal(got, []string{"answer 42", "answer 42", "look"}) {
		t.Fatalf("batch output = %q", got)
	}
	if got := s.GetHistoryEntries(); !slices.Equal(got, []input.Submission{input.Command(strings.Join(input.Command(source).Lines(), "\n"))}) {
		t.Fatalf("history = %+v", got)
	}
	if got := uiMock.drainEchoed(); len(got) != 4 {
		t.Fatalf("echo = %q", got)
	}
}

func TestLongSingleLineLuaExecutesOnce(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	payload := strings.Repeat("wrapped text ", 40)
	source := "/lua rune.send_raw('" + payload + "')"
	s.handleSubmission(input.Command(source))
	if got := net.drainSent(); !slices.Equal(got, []string{payload}) {
		t.Fatalf("long command sent %q", got)
	}
}

func TestSubmissionProcessesEachLineBeforeTheNextHook(t *testing.T) {
	for _, mode := range []input.SubmissionMode{input.ModeCommand, input.ModeVerbatim} {
		t.Run(mode.String(), func(t *testing.T) {
			s, net, _ := newTestSession(t)
			net.connected = true
			if err := s.engine.DoString("line ordering", `
				seen = {}
				rune.hooks.on("input", function(line, context)
					assert(not line:find("[\r\n]"))
					assert(#rune.history.get() == 0)
					seen[#seen + 1] = "input:" .. line
					if line == "skip" then return false end
					return line .. "!"
				end, {priority = 10})
				rune.hooks.on("echo", function(line)
					assert(#rune.history.get() == 0)
					seen[#seen + 1] = "echo:" .. line
				end, {priority = 10})
				local dispatch = rune.input._dispatch_line
				function rune.input._dispatch_line(line, mode)
					seen[#seen + 1] = "dispatch:" .. line
					return dispatch(line, mode)
				end
			`); err != nil {
				t.Fatal(err)
			}
			s.handleSubmission(input.Submission{Text: "first\nskip\nlast", Mode: mode})
			if got := net.drainSent(); !slices.Equal(got, []string{"first!", "last!"}) {
				t.Fatalf("sent %q", got)
			}
			assertSessionLua(t, s.engine, `assert(table.concat(seen, "|") == "input:first|echo:first!|dispatch:first!|input:skip|input:last|echo:last!|dispatch:last!")`)
			want := []input.Submission{{Text: "first!\nlast!", Mode: mode}}
			if got := s.GetHistoryEntries(); !slices.Equal(got, want) {
				t.Fatalf("history = %+v", got)
			}
		})
	}
}

func TestEarlierCommandCanInstallHookForFollowingLines(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	s.handleSubmission(input.Command("/lua rune.hooks.on('input', function(line) if line == 'look' then return 'score' end end)\nlook"))
	if got := net.drainSent(); !slices.Equal(got, []string{"score"}) {
		t.Fatalf("sent %q", got)
	}
}

func TestHistoryExpansionSeesScriptHistoryAdditions(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	s.AddToHistory("look")
	s.handleSubmission(input.Command("/lua rune.history.add('score')\n!\n!missing\nnorth\n!"))
	if got := net.drainSent(); !slices.Equal(got, []string{"score", "north", "score"}) {
		t.Fatalf("sent %q", got)
	}
	want := []input.Submission{input.Command("look"), input.Command("score"), input.Command("/lua rune.history.add('score')\nscore\nnorth\nscore")}
	if got := s.GetHistoryEntries(); !slices.Equal(got, want) {
		t.Fatalf("history = %+v", got)
	}
}

func TestSubmissionRejectsMultilineRewriteBeforeLaterHooks(t *testing.T) {
	for _, mode := range []input.SubmissionMode{input.ModeCommand, input.ModeVerbatim} {
		t.Run(mode.String(), func(t *testing.T) {
			s, net, _ := newTestSession(t)
			net.connected = true
			if err := s.engine.DoString("invalid rewrite", `
				rune.hooks.on("input", function(line) if line == "rewrite" then return "bad\ntext" end end, {priority = 10})
				late_seen = {}
                rune.hooks.on("input", function(line) late_seen[#late_seen + 1] = line end, {priority = 20})
			`); err != nil {
				t.Fatal(err)
			}
			s.handleSubmission(input.Submission{Text: "first\nrewrite\nlast", Mode: mode})
			assertSessionLua(t, s.engine, `assert(table.concat(late_seen, "|") == "first|last")`)
			if got := net.drainSent(); !slices.Equal(got, []string{"first", "last"}) {
				t.Fatalf("sent %q", got)
			}
		})
	}
}

func TestLargeInputRewritesReachDispatchAndHistory(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	assertSessionLua(t, s.engine, `rune.hooks.on("input", function() return string.rep("x", 300 * 1024) end)`)
	s.handleSubmission(input.Command("first\nsecond\nthird"))
	line := strings.Repeat("x", 300*1024)
	want := []string{line, line, line}
	if got := net.drainSent(); !slices.Equal(got, want) {
		t.Fatalf("large rewrites sent %d lines", len(got))
	}
	if got := s.GetHistoryEntries(); len(got) != 1 || got[0].Text != strings.Join(want, "\n") {
		t.Fatal("history did not preserve large rewrites")
	}
}

func TestQuitStopsFollowingSubmissionLines(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	s.handleSubmission(input.Command("north\n/quit\nsouth"))
	if got := net.drainSent(); !slices.Equal(got, []string{"north"}) {
		t.Fatalf("sent %q", got)
	}
}

func TestReloadWaitsUntilSubmissionFinishes(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("old alias", `rune.alias.exact("probe", "old")`); err != nil {
		t.Fatal(err)
	}
	s.handleSubmission(input.Command("/reload\nprobe"))
	if got := net.drainSent(); !slices.Equal(got, []string{"old"}) {
		t.Fatalf("sent before reload %q", got)
	}
	select {
	case event := <-s.internalEvents:
		s.handleInternalEvent(event)
	default:
		t.Fatal("reload was not queued")
	}
	s.handleSubmission(input.Command("probe"))
	if got := net.drainSent(); !slices.Equal(got, []string{"probe"}) {
		t.Fatalf("sent after reload %q", got)
	}
}

func TestFailingInputDispatcherIsNotRetried(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("broken dispatcher", `
		function rune.input._dispatch_line(text)
			rune.send_raw(text .. ":once")
			error("dispatch failed after send")
		end
	`); err != nil {
		t.Fatal(err)
	}

	s.handleSubmission(input.Command("north\nsouth"))
	if got, want := net.drainSent(), []string{"north:once"}; !slices.Equal(got, want) {
		t.Fatalf("dispatcher sends = %q, want no fallback duplicate %q", got, want)
	}
}

func TestCommandBatchContinuesAfterCommandErrors(t *testing.T) {
	for _, tc := range []struct{ name, setup, draft string }{
		{"unknown command", "", "north\n/missing\nsouth"},
		{"throwing command", `rune.command.add("broken", function() error("broken") end)`, "north\n/broken\nsouth"},
		{"throwing alias", `rune.alias.exact("broken", function() error("broken") end)`, "north\nbroken\nsouth"},
		{"invalid Lua", "", "north\n/lua invalid lua syntax\nsouth"},
		{"Lua runtime error", "", "north\n/lua error('broken')\nsouth"},
		{"alias recursion", `rune.alias.exact("loop", "loop")`, "north\nloop\nsouth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = true
			if err := s.engine.DoString(tc.name, tc.setup); err != nil {
				t.Fatal(err)
			}
			s.handleSubmission(input.Command(tc.draft))
			if got := net.drainSent(); !slices.Equal(got, []string{"north", "south"}) {
				t.Fatalf("sent %q", got)
			}
			if got := strings.Join(uiMock.drainPrinted(), "\n"); got == "" {
				t.Fatalf("missing error: %s", got)
			}
		})
	}
}

func TestCommandBatchHooksAndHistoryExpansion(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	s.AddToHistory("look\nscore\n/echo ignored")
	if err := s.engine.DoString("batch hooks", `
		seen = {}
		rune.hooks.on("input", function(line, context)
			assert(context.mode == "command")
			assert(#rune.history.get() == 1)
			seen[#seen + 1] = line
			if line == "rewrite" then return "say one;;two;!" end
		end, {priority = 50})
	`); err != nil {
		t.Fatal(err)
	}
	s.handleSubmission(input.Command("!\n/echo local\nrewrite\n\t\n!look"))
	if got := net.drainSent(); !slices.Equal(got, []string{"score", "say one;two", "!", "look"}) {
		t.Fatalf("sent %q", got)
	}
	assertSessionLua(t, s.engine, `assert(table.concat(seen, "|") == "score|/echo local|rewrite|look")`)
}

func TestHistoryReplaySelectsPhysicalLine(t *testing.T) {
	for _, tt := range []struct {
		name, previous, command string
		want                    []string
	}{
		{"multiline", "north\nkill rat\nlook", "!!", []string{"look"}},
		{"separator chain", "north;kill rat;look", "!!", []string{"north", "kill rat", "look"}},
		{"embedded replay", "north\nkill rat\nlook", "bla;!!;bla2", []string{"bla", "look", "bla2"}},
		{"prefix within batch", "north\nkill rat\nlook", "!ki", []string{"kill rat"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, net, _ := newTestSession(t)
			net.connected = true
			s.handleSubmission(input.Command(tt.previous))
			net.drainSent()
			if got := s.GetHistoryEntries(); !slices.Equal(got, []input.Submission{input.Command(tt.previous)}) {
				t.Fatalf("history = %+v, want whole submission", got)
			}
			s.handleSubmission(input.Command(tt.command))
			if got := net.drainSent(); !slices.Equal(got, tt.want) {
				t.Fatalf("replayed %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerbatimSubmissionWithMissingHooks(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	assertSessionLua(t, s.engine, "rune.hooks = nil")
	s.handleSubmission(input.Verbatim("first\r\n\r/quit\r"))
	if got := net.drainSent(); !slices.Equal(got, []string{"first", "", "/quit", ""}) {
		t.Fatalf("verbatim fallback sends = %q", got)
	}
	if s.backgroundCtx.Err() != nil {
		t.Fatal("verbatim /quit was executed")
	}
}
