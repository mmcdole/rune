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

func TestEditorBindingsReachUI(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `rune.unbind("ctrl+j")`)
	s.flushPresentation()
	if uiMock.pushedBinds().Hint("newline") != "Shift+Enter" {
		t.Fatal("editor bindings did not reach UI")
	}
}

func TestBindingEnableChangesReachUI(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `rune.bind("f1", "input.submit", {group="editing"})`)
	s.flushPresentation()
	for _, code := range []string{`rune.binds.get("f1"):disable()`, `rune.binds.get("f1"):enable(); rune.group.disable("editing")`} {
		assertSessionLua(t, s.engine, code)
		s.flushPresentation()
		if uiMock.pushedBinds()["f1"].Enabled {
			t.Fatal("inactive binding published as active")
		}
	}
	assertSessionLua(t, s.engine, `rune.group.enable("editing")`)
	s.flushPresentation()
	if !uiMock.pushedBinds()["f1"].Enabled {
		t.Fatal("enabled group did not update UI")
	}
}

func TestEmptyBindingsDoNotUseDegradedCoreDefaults(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	assertSessionLua(t, s.engine, `rune.binds.clear()`)
	s.pushBindsAndLayout()
	if len(uiMock.pushedBinds()) != 0 {
		t.Fatal("clearing bindings restored defaults")
	}
	assertSessionLua(t, s.engine, `rune.binds = nil`)
	s.pushBindsAndLayout()
	if !uiMock.pushedBinds().Matches("submit", "enter") {
		t.Fatal("unavailable core did not restore fallback editing")
	}
}

func TestNamedEditorAppliesOnlySuccessfulResults(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		ok           bool
	}{
		{"multiline", "north\neast\n\tkill goblin  ", true},
		{"empty", "", true},
		{"cancelled", "ignored", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, display := newTestSession(t)
			s.currentInput = "keep me"
			display.openEditorFn = func(initial string) (string, bool) {
				if initial != "UI draft" {
					t.Fatalf("editor received %q", initial)
				}
				return tc.result, tc.ok
			}
			s.handleUIEvent(ui.OpenEditorMsg{Text: "UI draft"})
			want := "keep me"
			if tc.ok {
				want = tc.result
			}
			if s.currentInput != want {
				t.Fatalf("draft %q, want %q", s.currentInput, want)
			}
		})
	}
}

func TestAppliedDraftReconcilesQueuedTypingWithoutNotifyingAgain(t *testing.T) {
	s, _, _ := newTestSession(t)
	assertSessionLua(t, s.engine, `
  changes = {}
  rune.hooks.on("input_changed", function(text) changes[#changes + 1] = text end)
  rune.input.set("script edit")
  rune.input.set_cursor(3)
 `)
	// The UI had already queued typing before it received the script's edit.
	s.handleUIEvent(ui.InputChangedMsg{Text: "older typing", Cursor: 12})
	// Its applied edit and cursor then arrive in UI order, without callbacks.
	s.handleUIEvent(ui.DraftAppliedMsg{Text: "script edit", Cursor: 11})
	s.handleUIEvent(ui.DraftAppliedMsg{Text: "script edit", Cursor: 3})
	if s.GetInput() != "script edit" || s.InputGetCursor() != 3 {
		t.Fatalf("mirror = %q at %d", s.GetInput(), s.InputGetCursor())
	}
	assertSessionLua(t, s.engine, `assert(table.concat(changes, "|") == "script edit|older typing")`)
}
