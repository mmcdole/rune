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
	})
}

// OnPresentationChange implements lua.Host.
func (s *Session) OnPresentationChange() {
	s.pushBindsAndLayout()
	s.pushBarUpdates()
}
