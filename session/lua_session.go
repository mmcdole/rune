package session

// The session store lives on the Session actor, not the Engine,
// precisely so it survives Engine.Init tearing the VM down on
// /reload. It is in-memory only: state that should survive a client
// restart belongs in rune.store (lua_store.go).

// SessionSet implements lua.Host.
func (s *Session) SessionSet(key, value string) {
	s.sessionStore[key] = value
}

// SessionGet implements lua.Host.
func (s *Session) SessionGet(key string) (string, bool) {
	v, ok := s.sessionStore[key]
	return v, ok
}

// SessionDelete implements lua.Host.
func (s *Session) SessionDelete(key string) {
	delete(s.sessionStore, key)
}
