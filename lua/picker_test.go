package lua

import "testing"

// TestPickerShowPassesOptions verifies rune.ui.picker.show marshals its
// opts (inline mode, dismiss_on_space) through to the host, and that
// dismiss_on_space defaults to off.
func TestPickerShowPassesOptions(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	script := `
		rune.ui.picker.show({
			items = {"/connect"},
			mode = "inline",
			dismiss_on_space = true,
			on_select = function() end,
		})
		rune.ui.picker.show({
			items = {"/connect"},
			mode = "inline",
			on_select = function() end,
		})
	`
	if err := engine.DoString("picker_opts", script); err != nil {
		t.Fatalf("picker.show failed: %v", err)
	}

	if len(host.PickerCalls) != 2 {
		t.Fatalf("expected 2 picker calls, got %d", len(host.PickerCalls))
	}
	if !host.PickerCalls[0].Inline || !host.PickerCalls[0].DismissOnSpace {
		t.Errorf("expected inline + dismiss_on_space on first call, got %+v", host.PickerCalls[0])
	}
	if host.PickerCalls[1].DismissOnSpace {
		t.Errorf("expected dismiss_on_space to default to false, got %+v", host.PickerCalls[1])
	}
}
