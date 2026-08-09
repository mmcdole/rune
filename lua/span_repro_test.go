package lua

import (
	"testing"

	"github.com/mmcdole/rune/text"
)

// Regression: a socket read ends after the first physical line and partway
// through the first continuation. The unterminated-prompt peek must remain
// visual-only; treating it as confirmed would flush the span at its header.
func TestSpanPreviewBetweenWrappedTellLines(t *testing.T) {
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

	// The session's local command echo is not a server line; skip it. The
	// first socket read yields the complete header plus a speculative peek
	// at the partial continuation.
	engine.OnOutput(text.NewLine("\x1b[1;32mYou tell Player: This is a long example tell that wraps onto"))
	engine.OnPromptPreview(text.NewLine("another physical"))

	// Later reads complete the continuation lines. The span should still
	// be open and collect them through its explicit ANSI terminator.
	engine.OnOutput(text.NewLine("another physical line and continues before"))
	engine.OnOutput(text.NewLine("ending on the final line\x1b[m"))

	// An unrelated tell still parses normally after the completed span.
	engine.OnOutput(text.NewLine("\x1b[1;32mPlayer tells you: A short reply\x1b[m"))

	assertLua(t, engine, `
		assert(#results == 2, "results: " .. #results ..
			(results[1] and (" first text: " .. tostring(results[1].text)) or ""))
		assert(results[1].kind == "tell-out", "kind1: " .. tostring(results[1].kind))
		assert(results[1].name == "Player", "name1: " .. tostring(results[1].name))
		local want = 'This is a long example tell that wraps onto ' ..
			'another physical line and continues before ending on the final line'
		assert(results[1].text == want, "text1: [" .. tostring(results[1].text) .. "]")
		assert(results[2].kind == "tell", "kind2: " .. tostring(results[2].kind))
		assert(results[2].text == "A short reply",
			"text2: [" .. tostring(results[2].text) .. "]")
	`)
}
