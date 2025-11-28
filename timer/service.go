package timer

import (
	"sync"
	"time"
)

// Event is sent when a timer fires.
type Event struct {
	ID        int
	Repeating bool
}

// Service manages timed wake-ups with full lifecycle ownership.
// It owns: ID generation, scheduling, repeating logic, cancellation.
// Uses fixed-interval semantics: repeating timers reschedule immediately on fire.
type Service struct {
	events chan<- Event
	timers map[int]*entry
	nextID int
	mu     sync.Mutex
}

type entry struct {
	interval time.Duration // 0 = one-shot, >0 = repeating
	cancel   func() bool   // time.Timer.Stop
}

// NewService creates a timer service that sends fired timer events.
func NewService(events chan<- Event) *Service {
	return &Service{
		events: events,
		timers: make(map[int]*entry),
	}
}

// After schedules a one-shot timer. Returns the timer ID.
func (s *Service) After(d time.Duration) int {
	return s.schedule(d, 0)
}

// Every schedules a repeating timer. Returns the timer ID.
func (s *Service) Every(d time.Duration) int {
	return s.schedule(d, d)
}

func (s *Service) schedule(d, interval time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	id := s.nextID

	t := time.AfterFunc(d, func() {
		s.fire(id)
	})

	s.timers[id] = &entry{
		interval: interval,
		cancel:   t.Stop,
	}

	return id
}

// fire sends the timer event and reschedules if repeating.
func (s *Service) fire(id int) {
	s.mu.Lock()
	e, ok := s.timers[id]
	if !ok {
		s.mu.Unlock()
		return // Cancelled before firing
	}

	repeating := e.interval > 0

	if repeating {
		// Fixed-interval: reschedule immediately
		t := time.AfterFunc(e.interval, func() {
			s.fire(id)
		})
		e.cancel = t.Stop
	} else {
		// One-shot: clean up
		delete(s.timers, id)
	}
	s.mu.Unlock()

	select {
	case s.events <- Event{ID: id, Repeating: repeating}:
	default:
		// Receiver shutting down or buffer full
	}
}

// Cancel stops a timer and removes it.
func (s *Service) Cancel(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.timers[id]; ok {
		e.cancel()
		delete(s.timers, id)
	}
}

// CancelAll stops all timers and clears the map.
func (s *Service) CancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.timers {
		e.cancel()
	}
	s.timers = make(map[int]*entry)
}
