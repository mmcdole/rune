package session

import (
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/ui"
)

// OnConfigChange implements lua.Host. Configuration publication is a
// typed, one-way boundary: it publishes immediately and never re-enters Lua
// or marks other snapshots stale.
func (s *Session) OnConfigChange(config lua.Config) {
	s.ui.UpdateConfig(ui.Config{
		KeepInput: config.KeepInput,
		Numpad:    config.Numpad,
		Mouse:     config.Mouse,
	})
}

// OnBindsChange implements lua.Host.
func (s *Session) OnBindsChange() {
	s.bindsDirty = true
}

// OnLayoutChange implements lua.Host. A callback that installs a layout and
// then hides a pane publishes one snapshot with the final visibility, never
// an intermediate one.
func (s *Session) OnLayoutChange() {
	s.layoutDirty = true
}

// OnBarsChange implements lua.Host.
func (s *Session) OnBarsChange() {
	s.barsDirty = true
}

// publishClientState mirrors clientState into rune.state and marks bars
// stale, since renderers commonly display it.
func (s *Session) publishClientState() {
	s.engine.UpdateState(s.clientState)
	s.barsDirty = true
}

// flushPresentation publishes each stale snapshot once. Each flag is cleared
// before its push: a bar renderer that marks a snapshot stale again leaves
// that change for the next event instead of looping here.
func (s *Session) flushPresentation() {
	if s.bindsDirty {
		s.bindsDirty = false
		s.pushBinds()
	}
	if s.layoutDirty {
		s.layoutDirty = false
		s.ui.UpdateLayout(s.engine.GetLayout())
	}
	if s.barsDirty {
		s.barsDirty = false
		s.pushBars()
	}
}

// pushBinds publishes the bind snapshot, falling back to Go's recovery
// defaults when the Lua bind module is unavailable.
func (s *Session) pushBinds() {
	bindings := s.engine.GetBindings()
	if bindings == nil {
		bindings = input.DefaultBindings()
	}
	s.ui.UpdateBinds(bindings)
}

// pushBars renders all Lua bars at the current width and publishes them.
func (s *Session) pushBars() {
	width := s.clientState.Width
	if width <= 0 {
		width = 80
	}
	// An empty successful snapshot is still state: it removes bars that
	// rendered content before and lets layout reclaim them. A failed pass
	// is nil and preserves the last good UI snapshot.
	if content := s.engine.RenderBars(width); content != nil {
		s.ui.UpdateBars(content)
	}
}
