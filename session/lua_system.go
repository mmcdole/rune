package session

import (
	"fmt"

	"github.com/drake/rune/event"
)

// Quit implements lua.SystemService.
func (s *Session) Quit() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Reload implements lua.SystemService.
// Must be deferred because it destroys the currently executing Lua state.
func (s *Session) Reload() {
	s.engine.CallHook("reloading")
	select {
	case s.events <- event.Event{
		Type: event.AsyncResult,
		Callback: func() {
			if err := s.boot(); err != nil {
				s.ui.Print(fmt.Sprintf("\033[31mReload Failed: %v\033[0m", err))
			} else {
				s.engine.CallHook("reloaded")
			}
		},
	}:
	default:
		s.ui.Print("\033[31mReload Failed: event queue full\033[0m")
	}
}

// Load implements lua.SystemService.
func (s *Session) Load(path string) {
	if path == "" {
		s.ui.Print("\033[31mLoad Failed: empty path\033[0m")
		return
	}

	if err := s.engine.DoFile(path); err != nil {
		s.ui.Print(fmt.Sprintf("\033[31mLoad Failed (%s): %v\033[0m", path, err))
		return
	}

	s.engine.CallHook("loaded", path)
}
