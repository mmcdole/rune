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

func TestMultilineLuaCommandPreservesSourceAndHistory(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	source := "/lua -- this comment must end at the newline\nlocal value = 40\n\trune.send_raw('answer ' .. (value + 2))"
	s.handleSubmission(input.Command(source))
	if got := net.drainSent(); !slices.Equal(got, []string{"answer 42"}) {
		t.Fatalf("multiline Lua output = %q", got)
	}
	if got := s.GetHistoryEntries(); !slices.Equal(got, []input.Submission{input.Command(source)}) {
		t.Fatalf("history = %+v", got)
	}
}
