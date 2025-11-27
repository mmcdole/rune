package timer

import "time"

// Scheduler manages delayed tasks by translating time into channel events.
// The receiver is responsible for executing the callback safely.
type Scheduler struct {
	out chan<- func()
}

// New creates a Scheduler that sends callbacks to the given channel.
func New(out chan<- func()) *Scheduler {
	return &Scheduler{out: out}
}

// Schedule asks to run 'job' after duration 'd'. Returns a cancel function.
func (s *Scheduler) Schedule(d time.Duration, job func()) (cancel func()) {
	t := time.AfterFunc(d, func() {
		s.out <- job
	})
	return func() { t.Stop() }
}
