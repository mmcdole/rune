package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/input"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

func TestPasteMessageRoutesAtomicallyToEditor(t *testing.T) {
	events := make(chan ui.UIEvent, 4)
	m := NewModel(events)

	next, _ := m.Update(tea.PasteMsg{Content: "say hello\nsay goodbye"})
	m = next.(*Model)

	if m.inputCtl.mode() != modeDraftEditor {
		t.Fatalf("paste mode = %v, want editor", m.inputCtl.mode())
	}
	if got := m.input.Value(); got != "say hello\nsay goodbye" {
		t.Fatalf("pasted input = %q", got)
	}
	changed, ok := (<-events).(ui.InputChangedMsg)
	if !ok || changed.Text != "say hello\nsay goodbye" {
		t.Fatalf("paste event = %#v, want one atomic input change", changed)
	}
}

func TestAcceptedSubmissionFollowsDraftChangeOnOneUIEventLane(t *testing.T) {
	events := make(chan ui.UIEvent, 4)
	m := NewModel(events)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "look"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = next.(*Model)

	changed, ok := (<-events).(ui.InputChangedMsg)
	if !ok || changed.Text != "look" {
		t.Fatalf("first event = %#v, want draft change to look", changed)
	}
	submitted, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok || submitted.Submission != input.Command("look") {
		t.Fatalf("second event = %#v, want command submission", submitted)
	}
	if submitted.NextDraft != "" {
		t.Fatalf("next draft = %q, want empty", submitted.NextDraft)
	}
	select {
	case event := <-events:
		t.Fatalf("accepted submission emitted redundant event %#v", event)
	default:
	}
}

func TestKeptSubmissionCarriesPostSubmitDraftInOneAcceptedEvent(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)

	next, _ := m.Update(ui.UpdateConfigMsg{KeepInput: true})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "north"})
	m = next.(*Model)

	// Drain the ordinary edit so the capacity-one queue can accept Enter.
	if changed, ok := (<-events).(ui.InputChangedMsg); !ok || changed.Text != "north" {
		t.Fatalf("draft event = %#v, want north", changed)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)

	got, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok {
		t.Fatalf("event = %T, want InputSubmittedMsg", got)
	}
	if got.Submission != input.Command("north") {
		t.Fatalf("submission = %+v, want north command", got.Submission)
	}
	if got.NextDraft != "north" {
		t.Fatalf("next draft = %q, want north", got.NextDraft)
	}
	if got := m.input.Value(); got != "north" || !m.input.Selected() {
		t.Fatalf("local input = %q selected=%v, want kept selection", got, m.input.Selected())
	}
	if got := m.output.Scrollback().Count(); got != 0 {
		t.Fatalf("warning rows = %d, want none", got)
	}
	select {
	case event := <-events:
		t.Fatalf("kept submit emitted a second event %#v", event)
	default:
	}
}

func TestFullUIEventQueueRejectsSubmissionWithoutLosingDraft(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "look"})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)

	if got := m.inputCtl.input.Value(); got != "look" {
		t.Fatalf("rejected submission changed draft to %q", got)
	}
	if got := m.output.Scrollback().Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.output.Scrollback().At(0)); !strings.Contains(warning, "Input not sent - engine lagging") {
		t.Fatalf("warning = %q", warning)
	}
	if _, ok := (<-events).(ui.InputChangedMsg); !ok {
		t.Fatal("queue no longer contains the accepted draft change")
	}
}

func TestFullUIEventQueueReportsDroppedOrdinaryEvent(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	events <- ui.InputChangedMsg{Text: "queued", Cursor: 6}
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(*Model)

	if got := m.output.Scrollback().Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.output.Scrollback().At(0)); !strings.Contains(warning, "UI event dropped - engine lagging") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestSetInputSubmissionMessageForcesVerbatimMode(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(ui.SetInputSubmissionMsg(input.Verbatim("one line;still data")))
	m = next.(*Model)

	if m.inputCtl.mode() != modeDraftEditor || !m.input.DraftEditorActive() {
		t.Fatal("explicit verbatim message did not enter editor")
	}
	if got := m.input.Value(); got != "one line;still data" {
		t.Fatalf("input = %q", got)
	}
}

func TestPushedDraftAcknowledgesStateWithoutReportingUserEdit(t *testing.T) {
	m := newBareModel(t)
	events := make(chan ui.UIEvent, 10)
	m.events = events
	m.Update(ui.SetInputMsg("script edit"))
	m.Update(ui.InputSetCursorMsg(3))
	for _, cursor := range []int{11, 3} {
		select {
		case event := <-events:
			if event != (ui.DraftAppliedMsg{Text: "script edit", Cursor: cursor}) {
				t.Fatalf("pushed editor state produced %#v", event)
			}
		default:
			t.Fatal("missing applied-state acknowledgment")
		}
	}
	if m.input.Value() != "script edit" || m.input.Position() != 3 {
		t.Fatalf("draft = %q at %d", m.input.Value(), m.input.Position())
	}
}
