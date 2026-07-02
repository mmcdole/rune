package session

import "time"

// TimerAfter implements lua.Host.
func (s *Session) TimerAfter(d time.Duration) int {
	return s.timer.After(d)
}

// TimerEvery implements lua.Host.
func (s *Session) TimerEvery(d time.Duration) int {
	return s.timer.Every(d)
}

// TimerCancel implements lua.Host.
func (s *Session) TimerCancel(id int) {
	s.timer.Cancel(id)
}

// TimerCancelAll implements lua.Host.
func (s *Session) TimerCancelAll() {
	s.timer.CancelAll()
}
