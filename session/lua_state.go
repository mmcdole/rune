package session

// OnConfigChange implements lua.Host.
func (s *Session) OnConfigChange() {
	s.pushBindsAndLayout()
	s.pushBarUpdates()
}
