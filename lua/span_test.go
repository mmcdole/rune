package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// Span triggers collect multi-line messages: the header match opens a
// span, following lines append, and the action fires once when
// span.to matches, span.max lines arrive, or an explicit boundary flushes. These
// tests drive Engine.OnOutput/OnPrompt line by line, the same way the
// session event loop does.

// assertLua runs a Lua assert() block and fails the test on error.
func assertLua(t *testing.T, engine *Engine, code string) {
	t.Helper()
	if err := engine.DoString("assert", code); err != nil {
		t.Fatal(err)
	}
}

func TestSpanCollectsAcrossLines(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
			fired = fired + 1
			got_name = m[1]
			got_text = ctx.text
			got_lines = #ctx.lines
			got_first = ctx.lines[1]:clean()
		end, { span = { to = "\\x1b\\[0?m\\s*$", raw = true, max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("\x1b[1;32mDrake tells you: part one of a"))
	engine.OnOutput(text.NewLine("long wrapped message"))
	engine.OnOutput(text.NewLine("that finally ends\x1b[m"))

	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(got_name == "Drake", "name: " .. tostring(got_name))
		assert(got_text == "part one of a long wrapped message that finally ends",
			"text: " .. tostring(got_text))
		assert(got_lines == 3, "lines: " .. tostring(got_lines))
		assert(got_first == "Drake tells you: part one of a", "first: " .. tostring(got_first))
	`)
}

func TestSpanSingleLineWhenHeaderMatchesTerminator(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { span = { to = "\\x1b\\[0?m\\s*$", raw = true } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("\x1b[1;32mDrake tells you: short one\x1b[m"))
	engine.OnOutput(text.NewLine("an unrelated following line"))

	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(got_text == "short one", "text: " .. tostring(got_text))
	`)
}

func TestSpanMaxFlush(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.starts("Quest log:", function(m, ctx)
			fired = fired + 1
			got_lines = #ctx.lines
			got_text = ctx.text
		end, { span = { to = "NEVER MATCHES ANYTHING", max = 3 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Quest log: two entries"))
	engine.OnOutput(text.NewLine("- slay the dragon"))
	assertLua(t, engine, `assert(fired == 0, "fired early: " .. fired)`)
	engine.OnOutput(text.NewLine("- find the ring"))

	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(got_lines == 3, "lines: " .. tostring(got_lines))
		assert(got_text == "Quest log: two entries - slay the dragon - find the ring",
			"text: " .. tostring(got_text))
	`)
}

func TestSpanMaxOnlyNoTerminator(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.exact("Your stats:", function(m, ctx)
			fired = fired + 1
			got_lines = #ctx.lines
		end, { span = { max = 2 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Your stats:"))
	engine.OnOutput(text.NewLine("STR 12  DEX 14"))

	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(got_lines == 2, "lines: " .. tostring(got_lines))
	`)
}

func TestSpanGagHidesEveryLine(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.trigger.starts("SPAM:", nil,
			{ gag = true, span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	for i, l := range []string{"SPAM: buy swords", "very cheap", "limited time END"} {
		if _, show := engine.OnOutput(text.NewLine(l)); show {
			t.Errorf("line %d (%q) should be gagged", i, l)
		}
	}
	if _, show := engine.OnOutput(text.NewLine("a normal line")); !show {
		t.Error("line after the span should display")
	}
}

func TestSpanPromptFlushesPartial(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		prompt_trigger = 0
		rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { span = { to = "NEVERMATCH", max = 8 } })
		rune.trigger.contains("100hp", function() prompt_trigger = prompt_trigger + 1 end,
			{ on = "prompt" })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Drake tells you: partial message"))
	engine.OnOutput(text.NewLine("still going"))
	assertLua(t, engine, `assert(fired == 0, "fired early")`)

	engine.OnPrompt(text.NewLine("100hp 50sp>"))

	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(got_text == "partial message still going", "text: " .. tostring(got_text))
		assert(prompt_trigger == 1, "opted-in prompt trigger should match")
	`)
}

func TestPartialGagsOpenSpanWithoutMutatingIt(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		got_text = nil
		rune.trigger.regex("^SECRET: (.+)$", function(matches, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { gag = true, span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	// A partial header is hidden but cannot open the span.
	if got := engine.OnPartial(text.NewLine("SECRET: first")); got != "" {
		t.Fatalf("gagged partial header displayed as %q", got)
	}
	assertLua(t, engine, `assert(fired == 0)`)

	// The same text opens the span only when it becomes a complete line.
	if _, show := engine.OnOutput(text.NewLine("SECRET: first")); show {
		t.Fatal("gagged span header should not display")
	}
	if got := engine.OnPartial(text.NewLine("continu")); got != "" {
		t.Fatalf("gagged partial continuation displayed as %q", got)
	}
	assertLua(t, engine, `assert(fired == 0, "partial continuation fired span")`)

	if _, show := engine.OnOutput(text.NewLine("continuation END")); show {
		t.Fatal("gagged span terminator should not display")
	}
	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(got_text == "first continuation END", "text: " .. tostring(got_text))
	`)
}

func TestExplicitSpanFlush(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.starts("Story:", function(matches, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { span = { to = "NEVER", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Story: unfinished"))
	engine.OnPartial(text.NewLine("HP:100>"))
	assertLua(t, engine, `assert(fired == 0, "partial flushed span")`)

	engine.FlushSpans()
	assertLua(t, engine, `
		assert(fired == 1, "flush did not fire span")
		assert(got_text == "Story: unfinished", "text: " .. tostring(got_text))
	`)
}

func TestSpanFlushHonorsRemovalDuringDispatch(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = {}
		rune.trigger.starts("First:", function()
			fired[#fired + 1] = "first"
			rune.trigger.remove("second")
		end, { priority = 10, span = { to = "NEVER", max = 8 } })
		rune.trigger.starts("Second:", function()
			fired[#fired + 1] = "second"
		end, { name = "second", priority = 20,
			span = { to = "NEVER", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("First: unfinished"))
	engine.OnOutput(text.NewLine("Second: unfinished"))
	engine.FlushSpans()

	assertLua(t, engine, `
		assert(#fired == 1 and fired[1] == "first",
			"removed span fired during flush: " .. table.concat(fired, ","))
	`)
}

func TestDiscardSpansDropsStateWithoutFiringOrRemovingTrigger(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		got_text = nil
		rune.trigger.starts("Story:", function(matches, ctx)
			fired = fired + 1
			got_text = ctx.text
		end, { once = true, span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Story: discarded"))
	engine.DiscardSpans()
	assertLua(t, engine, `
		assert(fired == 0, "discard fired span action")
		assert(#rune.trigger.list() == 1, "discard removed once-trigger")
	`)

	engine.OnOutput(text.NewLine("Story: retained"))
	engine.OnOutput(text.NewLine("ending END"))
	assertLua(t, engine, `
		assert(fired == 1, "trigger did not work after discard")
		assert(got_text == "Story: retained ending END",
			"discarded text leaked into next span: " .. tostring(got_text))
		assert(#rune.trigger.list() == 0, "once-trigger survived completed span")
	`)
}

func TestSpanNewHeaderRestarts(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		texts = {}
		rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
			texts[#texts + 1] = ctx.text
		end, { span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Drake tells you: first message"))
	engine.OnOutput(text.NewLine("with a wrap"))
	engine.OnOutput(text.NewLine("Soblak tells you: second message"))
	engine.OnOutput(text.NewLine("that terminates END"))

	assertLua(t, engine, `
		assert(#texts == 2, "fires: " .. #texts)
		assert(texts[1] == "first message with a wrap", "first: " .. tostring(texts[1]))
		assert(texts[2] == "second message that terminates END", "second: " .. tostring(texts[2]))
	`)
}

func TestSpanContinuationsStillHitOtherTriggers(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	// A higher-priority trigger rewrites a continuation line; the
	// display shows the rewrite and the span captures the rewritten
	// text (spans see what the trigger itself would have seen).
	if err := engine.DoString("setup", `
		rune.trigger.contains("dragon", function(m, ctx)
			return ctx.line:clean():gsub("dragon", "DRAGON")
		end, { priority = 10 })
		rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
			got_text = ctx.text
		end, { priority = 50, span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Drake tells you: beware the"))
	modified, show := engine.OnOutput(text.NewLine("dragon ahead END"))
	if !show || !strings.Contains(modified, "DRAGON") {
		t.Errorf("continuation should display rewritten, got %q show=%v", modified, show)
	}

	assertLua(t, engine, `
		assert(got_text == "beware the DRAGON ahead END", "text: " .. tostring(got_text))
	`)
}

func TestSpanOnceRemovesAfterCompletion(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.regex("^(\\w+) tells you: (.+)$", function()
			fired = fired + 1
		end, { name = "one-shot", once = true, span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Drake tells you: still open"))
	assertLua(t, engine, `
		assert(#rune.trigger.list() == 1, "should survive the header match")
	`)
	engine.OnOutput(text.NewLine("now done END"))
	engine.OnOutput(text.NewLine("Drake tells you: again END"))

	assertLua(t, engine, `
		assert(fired == 1, "fired: " .. fired)
		assert(#rune.trigger.list() == 0, "should be removed after completion")
	`)
}

func TestSpanActionReturnsIgnored(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	for _, ret := range []string{`false`, `"rewritten"`} {
		if err := engine.DoString("setup", `
			rune.trigger.clear()
			rune.trigger.starts("Notice:", function()
				return `+ret+`
			end, { span = { to = "END$", max = 8 } })
		`); err != nil {
			t.Fatal(err)
		}

		engine.OnOutput(text.NewLine("Notice: something"))
		modified, show := engine.OnOutput(text.NewLine("happened END"))
		if !show || modified != "happened END" {
			t.Errorf("return %s: terminator line should pass through, got %q show=%v",
				ret, modified, show)
		}
	}
}

func TestSpanInvalidToRaisesAtRegistration(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("setup", `
		rune.trigger.contains("x", nil, { span = { to = "([bad" } })
	`)
	if err == nil || !strings.Contains(err.Error(), "([bad") {
		t.Errorf("expected registration error naming the pattern, got %v", err)
	}
}

func TestSpanDroppedWhenTriggerRemovedMidSpan(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.starts("Story:", function()
			fired = fired + 1
		end, { name = "story", span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Story: once upon a time"))
	if err := engine.DoString("remove", `rune.trigger.remove("story")`); err != nil {
		t.Fatal(err)
	}
	engine.OnOutput(text.NewLine("the end END"))

	assertLua(t, engine, `assert(fired == 0, "removed trigger's span must not fire")`)
}

func TestSpanStringActionSendsOnCompletion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.trigger.regex("^(\\w+) invites you", "party accept %1",
			{ span = { to = "END$", max = 8 } })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Soblak invites you to a party"))
	if got := host.DrainNetworkCalls(); len(got) != 0 {
		t.Fatalf("string action must wait for completion, sent %v", got)
	}
	engine.OnOutput(text.NewLine("say yes to accept END"))

	if got := host.DrainNetworkCalls(); len(got) != 1 || got[0] != "party accept Soblak" {
		t.Errorf("expected one substituted send, got %v", got)
	}
}

func TestSpanRawTerminator(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		raw_fired = 0
		clean_fired = 0
		rune.trigger.starts("AAA", function() raw_fired = raw_fired + 1 end,
			{ span = { to = "\\x1b\\[m$", raw = true, max = 3 } })
		rune.trigger.starts("BBB", function() clean_fired = clean_fired + 1 end,
			{ span = { to = "\\x1b\\[m$", max = 3 } })
	`); err != nil {
		t.Fatal(err)
	}

	// The ANSI reset is only visible on the raw line; the clean-mode
	// span never sees it and runs to its max instead.
	engine.OnOutput(text.NewLine("AAA header"))
	engine.OnOutput(text.NewLine("done\x1b[m"))
	assertLua(t, engine, `assert(raw_fired == 1, "raw terminator should complete the span")`)

	engine.OnOutput(text.NewLine("BBB header"))
	engine.OnOutput(text.NewLine("done\x1b[m"))
	assertLua(t, engine, `assert(clean_fired == 0, "clean matching must not see the reset")`)
	engine.OnOutput(text.NewLine("third line hits max"))
	assertLua(t, engine, `assert(clean_fired == 1, "max should flush the clean-mode span")`)
}

// Regression for issue #103: a socket read ends after a tell header and
// partway through its first continuation. The partial is visual-only; the
// completed lines must still form one span.
func TestSpanPartialBetweenWrappedTellLines(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		results = {}
		local WRAP = { to = "\\x1b\\[0?m\\s*$", raw = true, max = 8 }
		rune.trigger.regex("^\\s*(\\w+) tells you: (.+)$", function(m, ctx)
			table.insert(results, { kind = "tell", name = m[1], text = ctx.text })
		end, { name = "tell-in", span = WRAP })
		rune.trigger.regex("^\\s*You tell (\\w+): (.+)$", function(m, ctx)
			table.insert(results, { kind = "tell-out", name = m[1], text = ctx.text })
		end, { name = "tell-out", span = WRAP })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("\x1b[1;32mYou tell Player: This is a long example tell that wraps onto"))
	engine.OnPartial(text.NewLine("another physical"))
	engine.OnOutput(text.NewLine("another physical line and continues before"))
	engine.OnOutput(text.NewLine("ending on the final line\x1b[m"))
	engine.OnOutput(text.NewLine("\x1b[1;32mPlayer tells you: A short reply\x1b[m"))

	assertLua(t, engine, `
		assert(#results == 2, "results: " .. #results ..
			(results[1] and (" first text: " .. tostring(results[1].text)) or ""))
		assert(results[1].kind == "tell-out", "kind1: " .. tostring(results[1].kind))
		assert(results[1].name == "Player", "name1: " .. tostring(results[1].name))
		local want = "This is a long example tell that wraps onto " ..
			"another physical line and continues before ending on the final line"
		assert(results[1].text == want, "text1: [" .. tostring(results[1].text) .. "]")
		assert(results[2].kind == "tell", "kind2: " .. tostring(results[2].kind))
		assert(results[2].text == "A short reply",
			"text2: [" .. tostring(results[2].text) .. "]")
	`)
}
