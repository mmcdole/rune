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
	if err := s.engine.DoString("setup", `
		reloading_fired = false
		rune.hooks.on("reloading", function() reloading_fired = true end)
	`); err != nil {
		t.Fatal(err)
	}
	for len(s.internalEvents) < cap(s.internalEvents) {
		s.internalEvents <- reloadRequested{}
	}

	s.Reload()

	printed := uiMock.drainPrinted()
	if !contains(printed, "Reload Failed: event queue full") {
		t.Fatalf("queue saturation was not reported: %v", printed)
	}
	if contains(printed, "Reloading scripts") {
		t.Errorf("announced a reload that was never queued: %v", printed)
	}
	assertSessionLua(t, s.engine, `assert(not reloading_fired, "reloading hook fired for a dropped reload")`)
}

func TestPostAfterSessionEndsIsRejectedEvenWithQueueSpace(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.cancelBackgroundWork()

	if s.postInternalEvent(s.backgroundCtx, httpFinished{}) {
		t.Fatal("posted an event on a cancelled context")
	}
	if len(s.internalEvents) != 0 {
		t.Fatalf("cancelled post enqueued %d events", len(s.internalEvents))
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

	s.cancelBackgroundWork()
	select {
	case ok := <-posted:
		if ok {
			t.Fatal("producer reported posting after Session stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("producer remained blocked after Session stopped")
	}
}
