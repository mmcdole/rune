package lua

import (
	"testing"

	"github.com/mmcdole/rune/text"
)

// The docs example on the rune.clipboard reference page: a span
// trigger collects a note, an alias copies it. Keep this in sync
// with website/src/content/docs/reference/api/clipboard.md.
func TestClipboardDocExampleCollectAndCopy(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	script := `
		local note = ""

		rune.trigger.starts("-- BEGIN NOTE", function(m, ctx)
		    local collected = {}
		    for _, line in ipairs(ctx.lines) do
		        collected[#collected + 1] = line:clean()
		    end
		    note = table.concat(collected, "\n")
		end, { span = { to = "^-- END NOTE", max = 40 } })

		rune.alias.exact("copynote", function()
		    rune.clipboard.set(note)
		end)
	`
	if err := engine.DoString("test", script); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for _, line := range []string{
		"-- BEGIN NOTE",
		"meet at the crossroads",
		"bring the key",
		"-- END NOTE",
	} {
		engine.OnOutput(text.NewLine(line))
	}
	engine.OnInput("copynote")

	want := "-- BEGIN NOTE\nmeet at the crossroads\nbring the key\n-- END NOTE"
	if len(host.ClipboardCalls) != 1 || host.ClipboardCalls[0] != want {
		t.Errorf("got clipboard calls %q, want [%q]", host.ClipboardCalls, want)
	}
}

func TestClipboardSetReachesHost(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("test", `rune.clipboard.set("note contents")`); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if len(host.ClipboardCalls) != 1 || host.ClipboardCalls[0] != "note contents" {
		t.Errorf("got clipboard calls %q, want [\"note contents\"]", host.ClipboardCalls)
	}
}
