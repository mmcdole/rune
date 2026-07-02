package session

// The persist store lives on the Session, not the Engine, precisely
// so it survives Engine.Init tearing the VM down on /reload. It is
// in-memory only: state that should survive a client restart belongs
// in a file under rune.config_dir.

// PersistSet implements lua.Host.
func (s *Session) PersistSet(key, value string) {
	s.persist[key] = value
}

// PersistGet implements lua.Host.
func (s *Session) PersistGet(key string) (string, bool) {
	v, ok := s.persist[key]
	return v, ok
}

// PersistDelete implements lua.Host.
func (s *Session) PersistDelete(key string) {
	delete(s.persist, key)
}
