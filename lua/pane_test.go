package lua

import (
	"strings"
	"testing"
)

// Pane visibility is placement state on the active layout tree: show, hide,
// and toggle flip the pane placement's gate, report whether the layout places
// the pane at all, and never reach the host's buffer operations.
func TestPaneVisibilityIsPlacementStateOnTheLayoutTree(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("pane layout", `
		rune.ui.layout({
			type = "column",
			children = {
				{ type = "pane", name = "output", border = "none" },
				{ type = "pane", name = "chat", hidden = true },
				{ type = "input" },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}
	host.DrainPresentationChanges()
	before := engine.GetLayout()

	assertLua(t, engine, `
		assert(rune.pane.is_visible("chat") == false)
		assert(rune.pane.is_visible("output") == true)
		assert(rune.pane.is_visible("missing") == nil)
	`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("visibility queries published %d presentation changes", got)
	}

	assertLua(t, engine, `assert(rune.pane.show("chat") == true)`)
	if got := host.DrainPresentationChanges(); got != 1 {
		t.Fatalf("show published %d presentation changes, want 1", got)
	}
	if !before.Root.Children[1].Hidden {
		t.Fatal("pane show mutated the previously returned layout snapshot")
	}
	if engine.GetLayout().Root.Children[1].Hidden {
		t.Fatal("pane remained hidden after show")
	}

	assertLua(t, engine, `assert(rune.pane.show("chat") == true)`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("idempotent show published %d presentation changes", got)
	}
	assertLua(t, engine, `
		assert(rune.pane.show("missing") == false)
		assert(rune.pane.hide("missing") == false)
		assert(rune.pane.toggle("missing") == false)
	`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("unplaced pane operations published %d presentation changes", got)
	}

	assertLua(t, engine, `assert(rune.pane.toggle("chat") == true)`)
	if got := host.DrainPresentationChanges(); got != 1 {
		t.Fatalf("toggle published %d presentation changes, want 1", got)
	}
	if !engine.GetLayout().Root.Children[1].Hidden {
		t.Fatal("toggle did not hide the pane")
	}

	assertLua(t, engine, `assert(rune.pane.hide("chat") == true)`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("idempotent hide published %d presentation changes", got)
	}

	if len(host.PaneCalls) != 0 {
		t.Fatalf("visibility operations reached host buffer API: %v", host.PaneCalls)
	}
}

func TestReservedOutputUsesTheOrdinaryPaneAPI(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	// The default layout places output, so its placement gate answers the
	// same visibility API as any declared pane.
	if err := engine.DoString("output pane API", `
		rune.pane.create("output")
		rune.pane.write("output", "line")
		assert(rune.pane.hide("output") == true)
		assert(rune.pane.is_visible("output") == false)
		assert(rune.pane.show("output") == true)
		assert(rune.pane.toggle("output") == true)
		assert(rune.pane.is_visible("output") == false)
		rune.pane.clear("output")
	`); err != nil {
		t.Fatal(err)
	}

	want := []struct{ Op, Name, Data string }{
		{"create", "output", ""},
		{"write", "output", "line"},
		{"clear", "output", ""},
	}
	if len(host.PaneCalls) != len(want) {
		t.Fatalf("got %d output pane calls, want %d: %v", len(host.PaneCalls), len(want), host.PaneCalls)
	}
	for i := range want {
		if host.PaneCalls[i] != want[i] {
			t.Errorf("call %d: got %v, want %v", i, host.PaneCalls[i], want[i])
		}
	}
}

func TestPaneAPIRejectsMissingAndNonStringNames(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	for _, call := range []string{
		`rune.pane.create()`,
		`rune.pane.write(1, "line")`,
		`rune.pane.toggle("")`,
		`rune.pane.show(false)`,
		`rune.pane.hide({})`,
		`rune.pane.is_visible(nil)`,
		`rune.pane.clear(nil)`,
		`rune.pane.scroll_up("", 1)`,
	} {
		err := engine.DoString("invalid pane name", call)
		if err == nil || !strings.Contains(err.Error(), "name must be a non-empty string") {
			t.Fatalf("%s error = %v, want non-empty string error", call, err)
		}
	}
	if len(host.PaneCalls) != 0 {
		t.Fatalf("invalid names reached host: %v", host.PaneCalls)
	}
}

func TestPaneScrollLinesMustBePositiveFiniteIntegers(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("valid pane scrolling", `
		rune.pane.scroll_up("output")
		rune.pane.scroll_down("output", 2)
	`); err != nil {
		t.Fatalf("valid pane scrolling failed: %v", err)
	}

	for _, lines := range []string{"0", "-1", "1.5", "math.huge", "0/0", `"2"`, "false"} {
		call := `rune.pane.scroll_up("output", ` + lines + `)`
		err := engine.DoString("invalid pane scroll distance", call)
		if err == nil || !strings.Contains(err.Error(), "lines must be a positive integer") {
			t.Fatalf("%s error = %v, want positive-integer error", call, err)
		}
	}
}
