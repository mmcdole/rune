package session

import (
	"testing"
	"time"
)

// awaitInternalEvent handles one asynchronous result exactly as the Session
// event loop would.
func awaitInternalEvent(t *testing.T, s *Session) {
	t.Helper()
	select {
	case event := <-s.internalEvents:
		s.handleInternalEvent(event)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for internal event")
	}
}

func TestStaleConnectCompletionCannotReplaceCurrentConnection(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.connectionID = 2
	s.clientState.Connected = true
	s.clientState.Address = "current.example:4000"

	s.handleInternalEvent(connectFinished{
		connectionID: 1,
		address:      "stale.example:4000",
	})

	if !s.clientState.Connected || s.clientState.Address != "current.example:4000" {
		t.Fatalf("stale dial replaced current state: %+v", s.clientState)
	}
}

func TestReloadReportsWhenInternalEventQueueIsFull(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	for len(s.internalEvents) < cap(s.internalEvents) {
		s.internalEvents <- reloadRequested{}
	}

	s.Reload()

	if printed := uiMock.drainPrinted(); !contains(printed, "Reload Failed: event queue full") {
		t.Fatalf("queue saturation was not reported: %v", printed)
	}
}

func TestInternalEventProducerStopsWhenSessionEnds(t *testing.T) {
	s, _, _ := newTestSession(t)
	for len(s.internalEvents) < cap(s.internalEvents) {
		s.internalEvents <- reloadRequested{}
	}

	posted := make(chan bool, 1)
	started := make(chan struct{})
	backgroundCtx := s.backgroundCtx
	go func() {
		close(started)
		posted <- s.postInternalEvent(backgroundCtx, httpFinished{})
	}()
	<-started

	s.stopBackgroundWork()
	select {
	case ok := <-posted:
		if ok {
			t.Fatal("producer reported posting after Session stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("producer remained blocked after Session stopped")
	}
}
