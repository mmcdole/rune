package lua

import (
	"fmt"
	"strings"
	"testing"
)

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

// TestPickerResultSettlesItsCallbackOnce verifies the Lua-side callback
// table: an accepted result runs on_select with the value, a cancelled one
// drops it, and any later result for the same or an unknown id is ignored.
func TestPickerResultSettlesItsCallbackOnce(t *testing.T) {
	tests := []struct {
		name    string
		results func(e *Engine, id int)
		want    string
	}{
		{name: "accepted", want: "picked", results: func(e *Engine, id int) {
			e.OnPickerResult(id, "picked", true)
		}},
		{name: "cancelled", want: "", results: func(e *Engine, id int) {
			e.OnPickerResult(id, "ignored", false)
		}},
		{name: "settled twice", want: "first", results: func(e *Engine, id int) {
			e.OnPickerResult(id, "first", true)
			e.OnPickerResult(id, "second", true)
		}},
		{name: "cancelled then accepted", want: "", results: func(e *Engine, id int) {
			e.OnPickerResult(id, "", false)
			e.OnPickerResult(id, "late", true)
		}},
		{name: "unknown id", want: "", results: func(e *Engine, id int) {
			e.OnPickerResult(id+1, "stray", true)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString("show", `
				selected = {}
				rune.ui.picker.show({
					items = {"a"},
					on_select = function(value) selected[#selected + 1] = value end,
				})
			`); err != nil {
				t.Fatal(err)
			}
			tt.results(engine, host.PickerCalls[0].CallbackID)
			if err := engine.DoString("check", fmt.Sprintf(`
				local got = table.concat(selected, ",")
				assert(got == %q, "selected = " .. got)
			`, tt.want)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPickerShowRequiresOnSelect(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	err := engine.DoString("show", `rune.ui.picker.show({ items = {"a"} })`)
	if err == nil || !strings.Contains(err.Error(), "on_select must be a function") {
		t.Fatalf("show without on_select: %v", err)
	}
	if len(host.PickerCalls) != 0 {
		t.Fatalf("picker shown without a callback: %+v", host.PickerCalls)
	}
}

// TestPickerResultFromBeforeReloadCannotSettleNewPicker covers a picker left
// open across /reload: when a new picker replaces it, the UI cancels the old
// id, which must not settle the new picker's callback.
func TestPickerResultFromBeforeReloadCannotSettleNewPicker(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	show := `
		selected = selected or {}
		rune.ui.picker.show({
			items = {"a"},
			on_select = function(value) selected[#selected + 1] = value end,
		})
	`
	if err := engine.DoString("before reload", show); err != nil {
		t.Fatal(err)
	}
	if err := engine.Init(); err != nil {
		t.Fatal(err)
	}
	loadTestCoreScripts(t, engine)
	if err := engine.DoString("after reload", show); err != nil {
		t.Fatal(err)
	}

	stale, current := host.PickerCalls[0].CallbackID, host.PickerCalls[1].CallbackID
	if stale == current {
		t.Fatalf("picker id %d reused across reload", current)
	}
	engine.OnPickerResult(stale, "", false)
	engine.OnPickerResult(current, "picked", true)
	if err := engine.DoString("check", `
		assert(#selected == 1 and selected[1] == "picked", "selected = " .. table.concat(selected, ","))
	`); err != nil {
		t.Fatal(err)
	}
}
