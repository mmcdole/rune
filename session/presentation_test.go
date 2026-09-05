package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mmcdole/rune/ui"
)

func TestWindowSizeChangeDispatchesHookWithStateInSync(t *testing.T) {
	s, _, _ := newTestSession(t)

	assertSessionLua(t, s.engine, `
		captured = nil
		rune.hooks.on("window_size_changed", function(w, h)
			captured = {
				w = w, h = h,
				w_type = type(w), h_type = type(h),
				state_w = rune.state.width, state_h = rune.state.height,
			}
		end)
	`)

	// The first reported size and later resizes share this path.
	s.handleUIEvent(ui.WindowSizeChangedMsg{Width: 120, Height: 40})

	assertSessionLua(t, s.engine, `
		assert(captured, "window_size_changed did not fire")
		assert(captured.w_type == "number" and captured.h_type == "number",
			"args must be numbers")
		assert(captured.w == 120 and captured.h == 40,
			"args " .. tostring(captured.w) .. "x" .. tostring(captured.h))
		assert(captured.state_w == 120 and captured.state_h == 40,
			"rune.state must already hold the new size during the callback")
	`)
}

func TestResizeHookLayoutChangeAppliesInSameCycle(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	assertSessionLua(t, s.engine, `
		rune.hooks.on("window_size_changed", function(w)
			if w < 80 then
				rune.ui.layout({ type = "column", children = {
					{ type = "pane", name = "output", border = "none" },
					{ type = "input" },
				} })
			end
		end)
	`)
	uiMock.drainLayoutPushes()

	s.handleUIEvent(ui.WindowSizeChangedMsg{Width: 60, Height: 40})

	if uiMock.drainLayoutPushes() == 0 {
		t.Error("layout change from a resize handler was not pushed during the resize cycle")
	}
	want := ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{
				Type: ui.LayoutTypePane, Name: ui.OutputPaneName,
				Border: ui.PaneBorderNone,
				Size:   ui.Fraction(1),
			},
			{
				Type: ui.LayoutTypeInput,
				Size: ui.AutoSize(),
			},
		},
	}}
	if got := uiMock.pushedLayout(); !reflect.DeepEqual(got, want) {
		t.Fatalf("pushed layout = %#v, want %#v", got, want)
	}
}

func TestBarRefreshPublishesOnlySuccessfulSnapshots(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	uiMock.drainBarPushes()

	assertSessionLua(t, s.engine, `rune.bars.clear()`)
	s.pushBarUpdates()

	count, bars := uiMock.drainBarPushes()
	if count != 1 {
		t.Fatalf("UpdateBars calls = %d, want one empty snapshot", count)
	}
	if len(bars) != 0 {
		t.Fatalf("UpdateBars payload = %#v, want no active bars", bars)
	}

	assertSessionLua(t, s.engine, `
		rune.bars._render_all = function()
			error("transient render failure")
		end
	`)
	uiMock.drainBarPushes()
	s.pushBarUpdates()
	if count, _ := uiMock.drainBarPushes(); count != 0 {
		t.Fatalf("failed bar render published %d snapshots, want none", count)
	}
}

func TestConfigSetPublishesRuntimeChangesAndOneFinalReloadSnapshot(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	uiMock.drainConfigPushes()

	if uiMock.pushedConfig().KeepInput {
		t.Fatal("keep_input must default off")
	}
	if uiMock.pushedConfig().Numpad {
		t.Fatal("numpad must default off")
	}
	if uiMock.pushedConfig().Mouse {
		t.Fatal("mouse must default off")
	}

	assertSessionLua(t, s.engine, `rune.config.set("keep_input", true)`)
	if !uiMock.pushedConfig().KeepInput {
		t.Fatal("keep_input=true did not reach the UI")
	}
	assertSessionLua(t, s.engine, `assert(rune.config.get("keep_input") == true)`)
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].KeepInput {
		t.Fatalf("runtime config pushes = %+v, want one keep_input=true", pushes)
	}

	assertSessionLua(t, s.engine, `rune.config.set("numpad", true)`)
	if !uiMock.pushedConfig().Numpad {
		t.Fatal("numpad=true did not reach the UI")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].Numpad {
		t.Fatalf("runtime config pushes = %+v, want one numpad=true", pushes)
	}

	assertSessionLua(t, s.engine, `rune.config.set("mouse", true)`)
	if !uiMock.pushedConfig().Mouse {
		t.Fatal("mouse=true did not reach the UI")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].Mouse {
		t.Fatalf("runtime config pushes = %+v, want one mouse=true", pushes)
	}

	// Parser settings use the same config publication path. Each update must
	// retain the current UI-facing values.
	assertSessionLua(t, s.engine, `
		rune.config.set("command_separator", "|")
		rune.config.set("history_character", "^")
	`)
	pushes := uiMock.drainConfigPushes()
	if len(pushes) != 2 {
		t.Fatalf("parser config pushes = %+v, want exactly two snapshots", pushes)
	}
	for i, push := range pushes {
		if !push.KeepInput || !push.Numpad || !push.Mouse {
			t.Fatalf("parser config push %d reset UI config: %+v", i, push)
		}
	}

	// Reload without an init.lua reverts to defaults.
	s.handleReloadRequested()
	if uiMock.pushedConfig().KeepInput {
		t.Fatal("reload did not reset keep_input to its default")
	}
	if uiMock.pushedConfig().Numpad {
		t.Fatal("reload did not reset numpad to its default")
	}
	if uiMock.pushedConfig().Mouse {
		t.Fatal("reload did not reset mouse to its default")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || pushes[0].KeepInput || pushes[0].Numpad || pushes[0].Mouse {
		t.Fatalf("default reload pushes = %+v, want exactly one final false snapshot", pushes)
	}
	assertSessionLua(t, s.engine, `
		assert(rune.config.get("command_separator") == ";")
		assert(rune.config.get("history_character") == "!")
	`)

	// Reload with an init.lua reapplies one final snapshot of all configured values.
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	initLua := `
rune.config.set("command_separator", "||")
rune.config.set("history_character", "?")
rune.config.set("keep_input", true)
rune.config.set("numpad", true)
rune.config.set("mouse", true)
`
	if err := os.WriteFile(initPath, []byte(initLua), 0o644); err != nil {
		t.Fatal(err)
	}
	s.handleReloadRequested()
	if !uiMock.pushedConfig().KeepInput {
		t.Fatal("reload did not reapply keep_input from init.lua")
	}
	if !uiMock.pushedConfig().Numpad {
		t.Fatal("reload did not reapply numpad from init.lua")
	}
	if !uiMock.pushedConfig().Mouse {
		t.Fatal("reload did not reapply mouse from init.lua")
	}
	if pushes := uiMock.drainConfigPushes(); len(pushes) != 1 || !pushes[0].KeepInput || !pushes[0].Numpad || !pushes[0].Mouse {
		t.Fatalf("configured reload pushes = %+v, want exactly one final true snapshot", pushes)
	}
	assertSessionLua(t, s.engine, `
		assert(rune.config.get("command_separator") == "||")
		assert(rune.config.get("history_character") == "?")
		assert(rune.config.get("keep_input") == true)
		assert(rune.config.get("numpad") == true)
		assert(rune.config.get("mouse") == true)
	`)
}

// TestPresentationChangesCoalesceIntoOnePushPerEvent: a bind that installs a
// layout and then flips a pane several times must reach the UI as one layout
// snapshot carrying the final gate, never as a sequence of intermediate
// snapshots.
func TestPresentationChangesCoalesceIntoOnePushPerEvent(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `
		rune.bind("f9", function()
			rune.ui.layout({ type = "column", children = {
				{ type = "pane", name = "group", size = 5 },
				{ type = "pane", name = "output", border = "none" },
				{ type = "input" },
			} })
			rune.pane.hide("group")
			rune.pane.show("group")
			rune.pane.hide("group")
		end)
	`)
	// Registering the bind outside a handler leaves the flag set. Start the
	// tested event clean so the count below is the callback's own doing.
	s.flushPresentation()
	uiMock.drainLayoutPushes()
	uiMock.drainBarPushes()

	s.handleUIEvent(ui.ExecuteBindMsg("f9"))

	if n := uiMock.drainLayoutPushes(); n != 1 {
		t.Fatalf("layout pushes during one bind = %d, want exactly one", n)
	}
	if n, _ := uiMock.drainBarPushes(); n != 1 {
		t.Fatalf("bar pushes during one bind = %d, want exactly one", n)
	}
	got := uiMock.pushedLayout()
	if len(got.Root.Children) != 3 || got.Root.Children[0].Name != "group" || !got.Root.Children[0].Hidden {
		t.Fatalf("pushed layout = %#v, want the group pane hidden", got)
	}
}

// TestBootPublishesOnePresentationSnapshot locks in the boot cost: core
// scripts register many binds, and none of them may push on its own.
func TestBootPublishesOnePresentationSnapshot(t *testing.T) {
	_, _, uiMock := newTestSession(t)
	if n := uiMock.drainLayoutPushes(); n != 1 {
		t.Fatalf("layout pushes during boot = %d, want exactly one", n)
	}
}

// TestTimerCallbacksFlushPresentationOnce covers the timer lane of the
// event loop: presentation changes made by a timer callback publish once when
// the timer handler returns.
func TestTimerCallbacksFlushPresentationOnce(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `
		rune.timer.after(0.01, function()
			rune.pane.hide("output")
			rune.pane.show("output")
		end)
	`)
	s.flushPresentation()
	uiMock.drainLayoutPushes()

	select {
	case evt := <-s.timerEvents:
		s.handleTimer(evt)
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire")
	}
	if n := uiMock.drainLayoutPushes(); n != 1 {
		t.Fatalf("layout pushes from one timer callback = %d, want exactly one", n)
	}
	if hidden, found := uiMock.pushedLayout().PaneHidden(ui.OutputPaneName); !found || hidden {
		t.Fatalf("pushed layout output hidden = %v, found = %v; want false, true", hidden, found)
	}
}
