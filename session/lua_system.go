package session

import (
	"fmt"

	"github.com/drake/rune/event"
	"github.com/drake/rune/text"
)

// Quit implements lua.Host.
func (s *Session) Quit() {
	s.ui.Quit()
}

// Reload implements lua.Host.
// Must be deferred because it destroys the currently executing Lua state.
// The send is non-blocking by necessity: Reload runs ON the session
// goroutine (called from inside a Lua dispatch), so blocking on the
// events channel here would deadlock the loop that drains it.
func (s *Session) Reload() {
	s.engine.CallHook("reloading")
	select {
	case s.events <- event.Event{
		Type: event.AsyncResult,
		Payload: event.Callback(func() {
			if err := s.boot(); err != nil {
				s.ui.Print(text.Red(fmt.Sprintf("Reload Failed: %v", err)))
			} else {
				s.engine.CallHook("reloaded")
			}
		}),
	}:
	default:
		s.ui.Print(text.Red("Reload Failed: event queue full"))
	}
}

// Load implements lua.Host.
func (s *Session) Load(path string) {
	if path == "" {
		s.ui.Print(text.Red("Load Failed: empty path"))
		return
	}

	if err := s.engine.DoFile(path); err != nil {
		s.ui.Print(text.Red(fmt.Sprintf("Load Failed (%s): %v", path, err)))
		return
	}

	s.engine.CallHook("loaded", path)
}

// RefreshBars forces an immediate bar refresh.
// Called from Lua when bar state changes and we don't want to wait for the ticker.
func (s *Session) RefreshBars() {
	s.pushBarUpdates()
}
