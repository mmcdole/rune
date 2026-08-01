package session

import (
	"fmt"

	"github.com/mmcdole/rune/text"
)

// Quit implements lua.Host.
func (s *Session) Quit() {
	s.ui.Quit()
}

// Reload implements lua.Host.
// Must be deferred because it destroys the currently executing Lua state.
// The send is non-blocking by necessity: Reload runs ON the session
// goroutine (called from inside a Lua dispatch), so blocking on the
// async-result channel here would deadlock the loop that drains it.
func (s *Session) Reload() {
	s.engine.CallHook("reloading")
	select {
	case s.asyncResults <- func() {
		if err := s.boot(); err != nil {
			s.ui.Print(text.Red(fmt.Sprintf("Reload Failed: %v", err)))
		} else {
			s.engine.CallHook("reloaded")
		}
	}:
	default:
		s.ui.Print(text.Red("Reload Failed: event queue full"))
	}
}

// RefreshBars forces an immediate bar refresh.
// Called from Lua when bar state changes and we don't want to wait for the ticker.
func (s *Session) RefreshBars() {
	s.pushBarUpdates()
}
