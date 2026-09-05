package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/input"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

func TestPasteMessageRoutesAtomicallyToComposer(t *testing.T) {
	events := make(chan ui.UIEvent, 4)
	m := NewModel(events)

	next, _ := m.Update(tea.PasteMsg{Content: "say hello\nsay goodbye"})
	m = next.(*Model)

	if m.inputCtl.mode() != modeCompose {
		t.Fatalf("paste mode = %v, want compose", m.inputCtl.mode())
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
	if got := m.output.buffer.Count(); got != 0 {
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
	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.output.buffer.At(0)); !strings.Contains(warning, "Input not sent - engine lagging") {
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

	if got := m.output.buffer.Count(); got != 1 {
		t.Fatalf("warning rows = %d, want exactly one", got)
	}
	if warning := runetext.StripANSI(m.output.buffer.At(0)); !strings.Contains(warning, "UI event dropped - engine lagging") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestOversizedVerbatimSubmissionIsRejectedAtomically(t *testing.T) {
	m := newBareModel(t)

	tooManyLines := input.Verbatim(strings.Repeat("\n", maxSubmissionLines))
	if m.submit(ui.InputSubmittedMsg{Submission: tooManyLines}) {
		t.Fatal("over-line-limit verbatim submission was accepted")
	}
	tooManyBytes := input.Verbatim(strings.Repeat("x", maxSubmissionBytes+1))
	if m.submit(ui.InputSubmittedMsg{Submission: tooManyBytes}) {
		t.Fatal("over-byte-limit verbatim submission was accepted")
	}
	tooManyCRLines := input.Verbatim(strings.Repeat("\r", maxSubmissionLines))
	if m.submit(ui.InputSubmittedMsg{Submission: tooManyCRLines}) {
		t.Fatal("over-line-limit bare-CR verbatim submission was accepted")
	}

	if got := m.output.buffer.Count(); got != 3 {
		t.Fatalf("warning count = %d, want 3", got)
	}
	for n := 0; n < m.output.buffer.Count(); n++ {
		if warning := m.output.buffer.At(n); !strings.Contains(warning, "Input not sent") {
			t.Fatalf("warning %d = %q", n, warning)
		}
	}
}

func TestVerbatimSubmissionAtLimitsIsAccepted(t *testing.T) {
	events := make(chan ui.UIEvent, 1)
	m := NewModel(events)
	text := strings.Repeat("x", maxSubmissionBytes-(maxSubmissionLines-1)) +
		strings.Repeat("\n", maxSubmissionLines-1)
	submission := input.Verbatim(text)

	if len(text) != maxSubmissionBytes {
		t.Fatalf("test setup bytes = %d, want %d", len(text), maxSubmissionBytes)
	}
	if !m.submit(ui.InputSubmittedMsg{Submission: submission}) {
		t.Fatal("at-limit verbatim submission was rejected")
	}
	got, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok || got.Submission != submission {
		t.Fatalf("queued event = %#v, want submission %+v", got, submission)
	}
}

func TestSetInputSubmissionMessageForcesVerbatimMode(t *testing.T) {
	m := newBareModel(t)
	next, _ := m.Update(ui.SetInputSubmissionMsg(input.Verbatim("one line;still data")))
	m = next.(*Model)

	if m.inputCtl.mode() != modeCompose || !m.input.IsComposing() {
		t.Fatal("explicit verbatim message did not enter composer")
	}
	if got := m.input.Value(); got != "one line;still data" {
		t.Fatalf("input = %q", got)
	}
}
