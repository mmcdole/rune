package session

import "time"

// TimerAfter implements lua.TimerService.
func (s *Session) TimerAfter(d time.Duration) int {
	return s.timer.After(d)
}

// TimerEvery implements lua.TimerService.
func (s *Session) TimerEvery(d time.Duration) int {
	return s.timer.Every(d)
}

// TimerCancel implements lua.TimerService.
func (s *Session) TimerCancel(id int) {
	s.timer.Cancel(id)
}

// TimerCancelAll implements lua.TimerService.
func (s *Session) TimerCancelAll() {
	s.timer.CancelAll()
}
