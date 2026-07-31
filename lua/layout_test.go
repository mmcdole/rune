package lua

import "testing"

// rune.ui.layout carries unknown table-entry keys to Go as an opaque
// option bag; name/height stay typed fields and never leak into it.
func TestLayoutEntryOptsPassThrough(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	script := `
		rune.ui.layout({
			top    = { { name = "chat", height = 10 } },
			bottom = { "input", { name = "separator", char = "=" }, "status" },
		})
	`
	if err := engine.DoString("test", script); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	layout := engine.GetLayout()

	if len(layout.Top) != 1 || len(layout.Bottom) != 3 {
		t.Fatalf("got %d top / %d bottom entries, want 1/3: %+v", len(layout.Top), len(layout.Bottom), layout)
	}

	chat := layout.Top[0]
	if chat.Name != "chat" || chat.Height != 10 || chat.Opts != nil {
		t.Errorf("chat entry = %+v, want name/height typed and no opts", chat)
	}

	sep := layout.Bottom[1]
	if sep.Name != "separator" || sep.Opts["char"] != "=" {
		t.Errorf("separator entry = %+v, want opts char %q", sep, "=")
	}
	if _, leaked := sep.Opts["name"]; leaked {
		t.Errorf("name leaked into opts: %+v", sep.Opts)
	}

	if plain := layout.Bottom[0]; plain.Name != "input" || plain.Opts != nil {
		t.Errorf("string entry = %+v, want bare name", plain)
	}
}
