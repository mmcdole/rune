package lua

import "testing"

// TestSearchEntryPoints verifies the Lua policy layer around scrollback
// search: the public wrapper, the Ctrl+F bind, and /find all reach the
// host's ShowSearch with the right query. The search itself (scanning,
// stepping, highlight) is UI-side and tested in ui/tui.
func TestSearchEntryPoints(t *testing.T) {
	cases := []struct {
		name  string
		run   func(e *Engine) error
		query string // expected ShowSearchMsg.Query
	}{
		{
			name:  "rune.ui.search with query",
			run:   func(e *Engine) error { return e.DoString("t", `rune.ui.search({ query = "thief" })`) },
			query: "thief",
		},
		{
			name:  "rune.ui.search without opts keeps last query",
			run:   func(e *Engine) error { return e.DoString("t", `rune.ui.search()`) },
			query: "",
		},
		{
			name:  "ctrl+f bind opens search",
			run:   func(e *Engine) error { return e.DoString("t", `rune.binds._dispatch("ctrl+f")`) },
			query: "",
		},
		{
			name:  "/find with pattern",
			run:   func(e *Engine) error { dispatchTestCommand(e, "/find the guard"); return nil },
			query: "the guard",
		},
		{
			name:  "/find bare reopens with last query",
			run:   func(e *Engine) error { dispatchTestCommand(e, "/find"); return nil },
			query: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			if err := tc.run(engine); err != nil {
				t.Fatalf("entry point failed: %v", err)
			}
			if len(host.SearchCalls) != 1 {
				t.Fatalf("expected 1 ShowSearch call, got %d", len(host.SearchCalls))
			}
			if got := host.SearchCalls[0].Query; got != tc.query {
				t.Errorf("Query = %q, want %q", got, tc.query)
			}
		})
	}
}

// TestFindCommandListed verifies /find appears in the command registry,
// which feeds both /help and the inline slash picker.
func TestFindCommandListed(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("find_listed", `
		for _, c in ipairs(rune.command.list()) do
			if c.name == "find" then return end
		end
		error("/find missing from the command registry")
	`)
	if err != nil {
		t.Error(err)
	}
}
