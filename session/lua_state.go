package session

import "github.com/mmcdole/rune/script"

// OnConfigChange implements lua.Host.
func (s *Session) OnConfigChange(executor script.Executor) {
	s.pushBindsAndLayoutIn(executor)
	s.pushBarUpdatesIn(executor)
}
