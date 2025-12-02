package session

import "github.com/drake/rune/lua"

// GetClientState implements lua.StateService.
func (s *Session) GetClientState() lua.ClientState {
	return s.clientState
}

// OnConfigChange implements lua.StateService.
func (s *Session) OnConfigChange() {
	s.pushBindsAndLayout()
	s.pushBarUpdates()
}
