package session

import (
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

func TestSetInputSubmissionForwardsExplicitMode(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	want := input.Verbatim("café;still data")

	s.SetInputSubmission(want)

	if len(uiMock.submissions) != 1 || uiMock.submissions[0] != want {
		t.Fatalf("explicit input updates = %+v, want [%+v]", uiMock.submissions, want)
	}
	if got := s.GetInput(); got != want.Text {
		t.Fatalf("Session input mirror = %q, want %q", got, want.Text)
	}
	if got, wantCursor := s.InputGetCursor(), len(want.Text); got != wantCursor {
		t.Fatalf("Session cursor mirror = %d, want %d", got, wantCursor)
	}
}

func TestInputCursorConvertsAtUIBoundary(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	s.handleUIEvent(ui.InputChangedMsg{Text: "café gob", Cursor: 8})
	if got, want := s.InputGetCursor(), len("café gob"); got != want {
		t.Fatalf("cursor after input change = %d, want %d", got, want)
	}

	s.handleUIEvent(ui.CursorMovedMsg{Cursor: 4})
	if got, want := s.InputGetCursor(), len("café"); got != want {
		t.Fatalf("cursor after UI move = %d, want %d", got, want)
	}

	s.InputSetCursor(4)
	if got, want := s.InputGetCursor(), 3; got != want {
		t.Fatalf("cursor inside UTF-8 sequence = %d, want %d", got, want)
	}
	if got, want := uiMock.inputCursor[len(uiMock.inputCursor)-1], 3; got != want {
		t.Fatalf("widget cursor = %d, want %d", got, want)
	}

	s.InputSetCursor(len("café"))
	if got, want := uiMock.inputCursor[len(uiMock.inputCursor)-1], 4; got != want {
		t.Fatalf("widget cursor after multibyte text = %d, want %d", got, want)
	}
}

func TestSearchStateIsIndependentFromScrollState(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.clientState.ScrollMode = "live"

	s.handleUIEvent(ui.SearchStateChangedMsg(true))
	if !s.clientState.SearchActive {
		t.Fatal("search-active UI event did not update client state")
	}
	if s.clientState.ScrollMode != "live" {
		t.Fatalf("search changed scroll mode to %q", s.clientState.ScrollMode)
	}

	s.handleUIEvent(ui.SearchStateChangedMsg(false))
	if s.clientState.SearchActive {
		t.Fatal("search-close UI event left client state active")
	}
}
