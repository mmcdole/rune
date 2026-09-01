package session

import (
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/ui"
)

// OnConfigChange implements lua.Host. Configuration publication is a
// typed, one-way boundary; it never re-enters Lua or invalidates unrelated
// presentation state.
func (s *Session) OnConfigChange(config lua.Config) {
	s.ui.UpdateConfig(ui.Config{
		KeepInput: config.KeepInput,
		Numpad:    config.Numpad,
		Mouse:     config.Mouse,
	})
}

// OnPresentationChange implements lua.Host. The push is deferred to the end
// of the Session event that is running, so a callback that installs a layout
// and then hides a pane publishes one layout snapshot with the final gates
// and never an intermediate one.
func (s *Session) OnPresentationChange() {
	s.presentationDirty = true
}

// flushPresentation publishes binds, layout, and bars once if anything
// changed. The flag is cleared before pushing: a bar renderer that changes
// presentation again leaves that change for the next event instead of
// looping here.
func (s *Session) flushPresentation() {
	if !s.presentationDirty {
		return
	}
	s.presentationDirty = false
	s.pushBindsAndLayout()
	s.pushBarUpdates()
}
