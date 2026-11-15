package lua

import (
	"fmt"
	"testing"

	"github.com/mmcdole/rune/input"
)

// rune._history.find walks history one entry at a time in Go, so Up and Down
// never copy the whole list into Lua.
func TestHistoryFind(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.HistoryEntries = []input.Submission{
		input.Command("say hi"), input.Command("kill rat"), input.Command("say bye"),
	}

	assertLua(t, engine, `
		local function check(want_pos, want_text, prefix, from, step)
			local pos, text, mode = rune._history.find(prefix, from, step)
			assert(pos == want_pos, ("find(%q, %d, %d) pos = %s, want %s"):format(
				prefix, from, step, tostring(pos), tostring(want_pos)))
			assert(text == want_text, ("find(%q, %d, %d) text = %s"):format(
				prefix, from, step, tostring(text)))
			if pos then assert(mode == "command", "mode = " .. tostring(mode)) end
		end

		-- Positions count from the newest entry (1 = "say bye").
		check(1, "say bye", "", 1, 1)       -- empty prefix: the entry at from
		check(3, "say hi", "", 3, -1)
		check(1, "say bye", "say", 1, 1)    -- older, from the newest
		check(3, "say hi", "say", 2, 1)     -- skips "kill rat"
		check(1, "say bye", "say", 2, -1)   -- newer
		check(2, "kill rat", "kill", 3, -1)
		check(nil, nil, "cast", 1, 1)       -- no match
		check(nil, nil, "", 4, 1)           -- past the oldest
		check(nil, nil, "", 0, -1)          -- past the newest

		assert(not pcall(rune._history.find, "", 1, 2), "step other than 1/-1 raises")
	`)
}

func TestHistoryNavigationResetsWhenEntryTrimmed(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.HistoryEntries = []input.Submission{
		input.Command("alpha"), input.Command("bravo"),
	}

	engine.HandleKeyBind("up")
	engine.HandleKeyBind("up")
	assertInput(t, host, "alpha")

	// History trimmed at its limit: the shown entry's position is gone.
	host.HistoryEntries = host.HistoryEntries[1:]
	engine.HandleKeyBind("down")
	assertInput(t, host, "alpha") // reset to the draft, input left as is
	engine.HandleKeyBind("up")
	assertInput(t, host, "alpha") // a new search from the draft "alpha": none
}

// BenchmarkHistoryKeys is the cost of one Up or Down press with a full
// 10,000-entry history.
func BenchmarkHistoryKeys(b *testing.B) {
	host := NewMockHost()
	for i := range 10000 {
		host.HistoryEntries = append(host.HistoryEntries, input.Command(fmt.Sprintf("cmd %d", i)))
	}
	engine := NewEngine(host)
	if err := engine.Init(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(engine.Close)
	if err := loadCoreScripts(engine); err != nil {
		b.Fatal(err)
	}

	b.Run("up-down", func(b *testing.B) {
		for b.Loop() {
			engine.HandleKeyBind("up")   // draft -> newest
			engine.HandleKeyBind("down") // newest -> draft
		}
	})
	b.Run("up-no-match", func(b *testing.B) {
		// Worst case: a prefix no entry has, so the whole history is scanned.
		if err := engine.DoString("draft.lua", `rune._input.restore("nomatch", "command")`); err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			engine.HandleKeyBind("up")
		}
	})
}
