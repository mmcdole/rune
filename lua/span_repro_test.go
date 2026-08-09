package lua

import (
	"testing"

	"github.com/mmcdole/rune/text"
)

// Repro: a socket read ends after the first physical line and partway through
// the first continuation. The unterminated-prompt peek must remain visual-only:
// treating it as a confirmed prompt flushes the span at the header and produces
// the truncated social-pane entry seen on Viking.
func TestSpanVikingCapturedStream(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	// Triggers registered exactly as viking/social.lua does, sharing one WRAP table.
	if err := engine.DoString("setup", `
		results = {}
		local WRAP = { to = "\\x1b\\[0?m\\s*$", raw = true, max = 8 }
		rune.trigger.regex("^\\s*(\\w+) tells you: (.+)$", function(m, ctx)
			table.insert(results, { kind = "tell", name = m[1], text = ctx.text })
		end, { name = "social-tell", span = WRAP })
		rune.trigger.regex("^\\s*You tell (\\w+): (.+)$", function(m, ctx)
			table.insert(results, { kind = "tell-out", name = m[1], text = ctx.text })
		end, { name = "social-tell-out", span = WRAP })
		rune.trigger.regex("^\\s*\\[Party\\]: (\\w+) says?: (.+)$", function(m, ctx)
			table.insert(results, { kind = "party", name = m[1], text = ctx.text })
		end, { name = "social-party", span = WRAP })
		rune.trigger.regex("^\\s*(\\w+) \\[(\\w+)\\]: (.+)$", function(m, ctx)
			table.insert(results, { kind = "channel", name = m[1], channel = m[2], text = ctx.text })
		end, { name = "social-channel", span = WRAP })
	`); err != nil {
		t.Fatal(err)
	}

	// The session's local command echo is not a server line; skip it. The
	// first socket read yields the complete header plus a speculative peek
	// at the partial continuation.
	engine.OnOutput(text.NewLine("\x1b[1;32mYou tell Storgrim: no, not all once. it was 11 \"pitches\".. although pitch"))
	engine.OnPromptPreview(text.NewLine("#1 was hilariously"))

	// Later reads complete the continuation lines. The span should still
	// be open and collect them through its explicit ANSI terminator.
	engine.OnOutput(text.NewLine("#1 was hilariously the hardest.. and some of the pitches were simple.. like"))
	engine.OnOutput(text.NewLine("walking through a mine shaft was one of the 11, which was trivial\x1b[m"))

	// An unrelated tell still parses normally after the completed span.
	engine.OnOutput(text.NewLine(" \x1b[m\x1b[1;32mStorgrim tells you: You must tell me 'buy <lot>' or 'list'\x1b[m"))

	assertLua(t, engine, `
		assert(#results == 2, "results: " .. #results ..
			(results[1] and (" first text: " .. tostring(results[1].text)) or ""))
		assert(results[1].kind == "tell-out", "kind1: " .. tostring(results[1].kind))
		assert(results[1].name == "Storgrim", "name1: " .. tostring(results[1].name))
		local want = 'no, not all once. it was 11 "pitches".. although pitch ' ..
			'#1 was hilariously the hardest.. and some of the pitches were simple.. like ' ..
			'walking through a mine shaft was one of the 11, which was trivial'
		assert(results[1].text == want, "text1: [" .. tostring(results[1].text) .. "]")
		assert(results[2].kind == "tell", "kind2: " .. tostring(results[2].kind))
		assert(results[2].text == "You must tell me 'buy <lot>' or 'list'",
			"text2: [" .. tostring(results[2].text) .. "]")
	`)
}
