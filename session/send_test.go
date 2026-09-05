package session

import (
	"slices"
	"testing"

	"github.com/mmcdole/rune/network"
)

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
