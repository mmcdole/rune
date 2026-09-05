package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mmcdole/rune/ui"
)

// Reload must be deferred through the event queue - it tears down the
// VM that is executing the /reload command - and must leave a working
// scripting environment behind.
func TestReloadIsDeferredAndRebuildsVM(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("setup", `rune.alias.exact("n", "north")`); err != nil {
		t.Fatal(err)
	}

	if err := s.engine.DoString("request reload", `rune.reload()`); err != nil {
		t.Fatal(err)
	}

	// Reload is queued, not executed inline.
	select {
	case event := <-s.internalEvents:
		s.handleInternalEvent(event)
	default:
		t.Fatal("reload did not queue an internal event")
	}

	if printed := uiMock.drainPrinted(); !contains(printed, "Scripts reloaded") {
		t.Errorf("expected reload completion notice, got %v", printed)
	}

	// The old VM's registrations are gone; the new VM works
	userInput(s, "n")
	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "n" {
		t.Errorf("expected alias gone after reload, got %v", sent)
	}
	if err := s.engine.DoString("check", `assert(rune.hooks ~= nil)`); err != nil {
		t.Errorf("scripting broken after reload: %v", err)
	}
}

func TestReloadRestoresDimensionsWithoutSyntheticResize(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.handleUIEvent(ui.WindowSizeChangedMsg{Width: 100, Height: 30})

	initLua := `
		width_at_load = rune.state.width
		height_at_load = rune.state.height
		resize_fired = false
		rune.hooks.on("window_size_changed", function() resize_fired = true end)
	`
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	if err := os.WriteFile(initPath, []byte(initLua), 0o644); err != nil {
		t.Fatal(err)
	}

	s.handleReloadRequested()

	assertSessionLua(t, s.engine, `
		assert(width_at_load == 100 and height_at_load == 30,
			"init.lua must see the restored size, got " ..
			tostring(width_at_load) .. "x" .. tostring(height_at_load))
		assert(rune.state.width == 100 and rune.state.height == 30)
		assert(resize_fired == false, "reload must not fire a synthetic resize")
	`)
}
