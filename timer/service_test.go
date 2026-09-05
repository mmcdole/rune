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
