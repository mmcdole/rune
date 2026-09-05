package timer

import (
	"testing"
	"testing/synctest"
	"time"
)

func waitEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timer event")
		return Event{}
	}
}

func TestAfterFiresOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 16)
		s := NewService(events)
		defer s.Stop()

		id := s.After(time.Millisecond)
		ev := waitEvent(t, events)
		if ev.ID != id || ev.Repeating {
			t.Errorf("want one-shot event id=%d, got %+v", id, ev)
		}

		select {
		case ev := <-events:
			t.Errorf("one-shot fired twice: %+v", ev)
		case <-time.After(20 * time.Millisecond):
		}
	})
}

func TestEveryRepeats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 16)
		s := NewService(events)
		defer s.Stop()

		id := s.Every(time.Millisecond)
		for i := 0; i < 3; i++ {
			ev := waitEvent(t, events)
			if ev.ID != id || !ev.Repeating {
				t.Fatalf("want repeating event id=%d, got %+v", id, ev)
			}
		}
	})
}

func TestCancelStopsTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 16)
		s := NewService(events)
		defer s.Stop()

		id := s.After(50 * time.Millisecond)
		s.Cancel(id)

		select {
		case ev := <-events:
			t.Errorf("cancelled timer fired: %+v", ev)
		case <-time.After(100 * time.Millisecond):
		}
	})
}

func TestCancelAll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 16)
		s := NewService(events)
		defer s.Stop()

		s.After(50 * time.Millisecond)
		s.Every(50 * time.Millisecond)
		s.CancelAll()

		select {
		case ev := <-events:
			t.Errorf("timer fired after CancelAll: %+v", ev)
		case <-time.After(100 * time.Millisecond):
		}
	})
}

// A one-shot fire must be delivered even when the buffer is
// momentarily full: the fire goroutine blocks until the consumer
// drains, rather than dropping the event.
func TestOneShotSurvivesFullBuffer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 1)
		s := NewService(events)
		defer s.Stop()

		// Occupy the only buffer slot so the fire cannot complete instantly.
		events <- Event{ID: -1}

		id := s.After(time.Millisecond)

		// Advance past the deadline, then wait until delivery is blocked.
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if got := s.Remaining(id); got != 0 {
			t.Fatalf("remaining during blocked one-shot delivery = %v, want zero", got)
		}

		if got := waitEvent(t, events); got.ID != -1 {
			t.Fatalf("want placeholder event first, got %+v", got)
		}
		if got := waitEvent(t, events); got.ID != id {
			t.Fatalf("want blocked one-shot delivered after drain, got %+v", got)
		}
	})
}

// Stop must unblock a fire goroutine stuck on a full buffer so it
// does not leak when the consumer goes away.
func TestStopUnblocksPendingFire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 1)
		s := NewService(events)

		events <- Event{ID: -1} // fill the buffer, no consumer
		defer s.Stop()
		s.After(time.Millisecond)
		time.Sleep(time.Millisecond)
		synctest.Wait() // the fire goroutine is blocked on the full buffer

		s.Stop()
		// Test waits for every goroutine in the bubble to exit. A leaked fire
		// goroutine fails with a deadlock even if Stop itself returns promptly.
	})
}

func TestRemainingTracksNextWakeup(t *testing.T) {
	for _, tc := range []struct {
		name      string
		repeating bool
	}{
		{name: "one-shot"},
		{name: "repeating", repeating: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				events := make(chan Event, 1)
				s := NewService(events)
				defer s.Stop()
				var id int
				if tc.repeating {
					id = s.Every(10 * time.Second)
				} else {
					id = s.After(10 * time.Second)
				}
				if got := s.Remaining(id); got != 10*time.Second {
					t.Fatalf("initial remaining = %v, want 10s", got)
				}
				time.Sleep(2500 * time.Millisecond)
				if got := s.Remaining(id); got != 7500*time.Millisecond {
					t.Fatalf("remaining after 2.5s = %v, want 7.5s", got)
				}
				time.Sleep(7500 * time.Millisecond)
				synctest.Wait()
				// The consumer has not handled the wake-up yet. The countdown
				// is tied to scheduling, not callback dispatch or observation.
				want := time.Duration(0)
				if tc.repeating {
					want = 10 * time.Second
				}
				if got := s.Remaining(id); got != want {
					t.Fatalf("remaining with queued wake-up = %v, want %v", got, want)
				}
				if ev := waitEvent(t, events); ev.ID != id {
					t.Fatalf("wake-up = %+v, want id %d", ev, id)
				}
			})
		})
	}
}

func TestRemainingAdvancesWhenRepeatingWakeupIsDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan Event, 1)
		events <- Event{ID: -1}
		s := NewService(events)
		defer s.Stop()
		id := s.Every(10 * time.Second)

		time.Sleep(22 * time.Second)
		synctest.Wait()
		if got := s.Remaining(id); got != 8*time.Second {
			t.Fatalf("remaining after two dropped ticks = %v, want 8s", got)
		}
		if got := <-events; got.ID != -1 {
			t.Fatalf("full buffer was changed: %+v", got)
		}
	})
}

func TestRemainingIsZeroAfterCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel func(*Service, int)
	}{
		{"cancel", func(s *Service, id int) { s.Cancel(id) }},
		{"cancel all", func(s *Service, _ int) { s.CancelAll() }},
		{"stop", func(s *Service, _ int) { s.Stop() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewService(make(chan Event, 2))
				defer s.Stop()
				ids := []int{s.After(time.Minute), s.Every(time.Minute)}
				for _, id := range ids {
					tc.cancel(s, id)
				}
				for _, id := range append(ids, -1) {
					if got := s.Remaining(id); got != 0 {
						t.Fatalf("remaining for unscheduled id %d = %v, want zero", id, got)
					}
				}
			})
		})
	}
}

// Model a deadline that passed before the timer goroutine could service it.
func TestRemainingClampsOverdueWakeup(t *testing.T) {
	s := &Service{timers: map[int]*entry{
		1: {deadline: time.Now().Add(-time.Second)},
	}}
	if got := s.Remaining(1); got != 0 {
		t.Fatalf("remaining for overdue wake-up = %v, want zero", got)
	}
}
