package lua

import "testing"

// rune.pane.show/hide are idempotent setters over one Go primitive;
// what can silently break is the wrapper-to-flag mapping, so pin it.
func TestPaneShowHideReachHostWithVisibility(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	script := `
		rune.pane.create("chat")
		rune.pane.show("chat")
		rune.pane.hide("chat")
		rune.pane.toggle("chat")
	`
	if err := engine.DoString("test", script); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	want := []struct{ Op, Name, Data string }{
		{"create", "chat", ""},
		{"set_visible", "chat", "true"},
		{"set_visible", "chat", "false"},
		{"toggle", "chat", ""},
	}
	if len(host.PaneCalls) != len(want) {
		t.Fatalf("got %d pane calls, want %d: %v", len(host.PaneCalls), len(want), host.PaneCalls)
	}
	for i, w := range want {
		if host.PaneCalls[i] != w {
			t.Errorf("call %d: got %v, want %v", i, host.PaneCalls[i], w)
		}
	}
}
