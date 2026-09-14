package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

func TestDraftModeTogglePreservesEditingState(t *testing.T) {
	for _, draft := range []string{"look;east", "/lua -- keep the newline\nrune.echo('hello')"} {
		t.Run(draft, func(t *testing.T) {
			h := newControllerHarness()
			h.ctl.HandlePaste(draft)
			h.ctl.input.SetCursor(3)
			h.ctl.input.SelectAll()
			originalMode := h.ctl.input.SubmissionMode()
			eventCount := len(h.events)
			for range 2 {
				h.ctl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
				if !h.ctl.input.IsComposing() {
					t.Fatal("explicit mode switch must open and retain the composer")
				}
				if h.ctl.input.Value() != draft || h.ctl.input.Position() != 3 || !h.ctl.input.Selected() {
					t.Fatal("mode switch changed text, cursor, or selection")
				}
			}
			if h.ctl.input.SubmissionMode() != originalMode || len(h.submitted) != 0 || len(h.events) != eventCount {
				t.Fatal("toggle must change only draft interpretation")
			}
		})
	}
}

func TestExplicitCommandModeSurvivesStructuredEdits(t *testing.T) {
	h := newControllerHarness()
	h.ctl.HandlePaste("/lua -- comment\n")
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	h.ctl.HandlePaste("\trune.echo('hello')\n")
	h.ctl.SetText("/lua -- comment\nrune.echo('edited')")
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	want := input.Command("/lua -- comment\nrune.echo('edited')")
	if len(h.submitted) != 1 || h.submitted[0] != want {
		t.Fatalf("submissions = %+v", h.submitted)
	}
	if h.ctl.input.Value() != "" || h.ctl.input.SubmissionMode() != input.ModeCommand {
		t.Fatal("fresh draft must start in Command mode")
	}
	h.ctl.HandlePaste("one\ntwo")
	if h.ctl.input.SubmissionMode() != input.ModeVerbatim {
		t.Fatal("fresh structured paste must default to Verbatim")
	}
}

func TestAltEnterDoesNotOverrideSubmissionMode(t *testing.T) {
	h := newControllerHarness()
	h.ctl.HandlePaste("first\nsecond")
	h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if len(h.submitted) != 0 || h.ctl.input.SubmissionMode() != input.ModeVerbatim {
		t.Fatal("Alt+Enter must not submit or change mode")
	}
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 1 || h.submitted[0] != input.Verbatim("first\nsecond") {
		t.Fatalf("submit = %+v", h.submitted)
	}
}

func TestMultilineCommandHistoryRestoresModeAndNavigation(t *testing.T) {
	h := newControllerHarness()
	h.bound["down"] = true
	draft := input.Command("/lua -- comment\nrune.echo('hello')")
	h.ctl.SetSubmission(draft)
	h.ctl.HandleKey(keyPress(tea.KeyDown))
	if got := h.events[len(h.events)-1]; got != ui.ExecuteBindMsg("down") {
		t.Fatalf("history boundary event = %#v", got)
	}
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 1 || h.submitted[0] != draft {
		t.Fatalf("recalled submission = %+v", h.submitted)
	}
}

func TestKeepMultilineCommandSelectsAndReplacesWholeDraft(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetKeepOnSubmit(true)
	draft := input.Command("/lua -- comment\nrune.echo('hello')")
	h.ctl.SetSubmission(draft)
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if !h.ctl.input.Selected() || h.nextDrafts[0] != draft.Text {
		t.Fatal("kept command was not selected")
	}
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 2 || h.submitted[1] != draft {
		t.Fatal("Enter did not resend retained command")
	}
	h.ctl.HandleKey(textPress("look"))
	if h.ctl.input.Value() != "look" || h.ctl.input.Selected() {
		t.Fatal("typing did not replace whole multiline command")
	}
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	h.ctl.HandlePaste("one\ntwo")
	if h.ctl.input.Value() != "one\ntwo" || h.ctl.input.SubmissionMode() != input.ModeVerbatim {
		t.Fatal("pasting over kept command did not start a fresh draft")
	}
}

func TestModeShortcutsRespectOverlayCaptureAndAltGr(t *testing.T) {
	h := newControllerHarness()
	h.ctl.HandlePaste("look")
	h.ctl.HandleKey(altGrPress('v', "v"))
	if h.ctl.input.Value() != "lookv" || h.ctl.input.SubmissionMode() != input.ModeCommand {
		t.Fatal("AltGr text was treated as toggle")
	}
	h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "modal"})
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if len(h.submitted) != 0 || h.ctl.input.SubmissionMode() != input.ModeCommand {
		t.Fatal("modal keys affected underlying draft")
	}
	h.ctl.HandleKey(keyPress(tea.KeyEsc))
	h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "inline", Inline: true})
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	if h.ctl.input.PickerActive() || h.ctl.input.SubmissionMode() != input.ModeVerbatim {
		t.Fatal("inline toggle did not settle picker and change mode")
	}
}

func TestRejectedCommandPreservesDraftAndCanBeSentVerbatim(t *testing.T) {
	events := make(chan ui.UIEvent, 20)
	m := NewModel(events)
	m.inputCtl.HandlePaste("north\x1blook")
	m.inputCtl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	m.inputCtl.HandleKey(keyPress(tea.KeyEnter))
	if m.input.Value() != "north\x1blook" || m.input.SubmissionMode() != input.ModeCommand {
		t.Fatal("invalid command lost draft")
	}
	if m.output.buffer.Count() == 0 || !strings.Contains(m.output.buffer.At(0), "Command not run") {
		t.Fatal("missing rejection feedback")
	}
	for len(events) > 0 {
		if _, ok := (<-events).(ui.InputSubmittedMsg); ok {
			t.Fatal("invalid command was queued")
		}
	}
	m.inputCtl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	m.inputCtl.HandleKey(keyPress(tea.KeyEnter))
	msg, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok || msg.Submission != input.Verbatim("north\x1blook") {
		t.Fatalf("verbatim retry = %#v", msg)
	}
}

func TestComposerEditorHintTracksBindingUpdates(t *testing.T) {
	m := newBareModel(t)
	m.input.SetSize(100, 0)
	m.inputCtl.HandlePaste("first\nsecond")
	for _, available := range []bool{false, true, false} {
		m.Update(ui.UpdateBindsMsg{"ctrl+e": {Action: "input.open_editor", Enabled: available}})
		var labels string
		for _, rule := range m.input.Rules(100, 4) {
			for _, label := range rule.Labels {
				labels += label.Text
			}
		}
		if strings.Contains(labels, "Ctrl+E editor") != available {
			t.Fatalf("editor hint with binding=%v: %q", available, labels)
		}
	}
}

func TestEscapeConfirmationDoesNotConsumeSubmit(t *testing.T) {
	h := newControllerHarness()
	h.ctl.HandlePaste("first\nsecond")
	h.ctl.HandleKey(keyPress(tea.KeyEsc))
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 1 || h.submitted[0] != input.Verbatim("first\nsecond") {
		t.Fatalf("Enter after Escape = %+v", h.submitted)
	}
	if h.ctl.input.IsComposing() || h.ctl.input.Value() != "" {
		t.Fatal("accepted submission retained composer")
	}
}

func TestComposerWrapDoesNotSplitSlashCommand(t *testing.T) {
	for _, prefix := range []string{"/lua rune.echo('", "/echo "} {
		t.Run(prefix, func(t *testing.T) {
			h := newControllerHarness()
			h.ctl.input.SetSize(30, 0)
			draft := prefix + strings.Repeat("long command ", 30)
			if strings.HasPrefix(prefix, "/lua") {
				draft += "')"
			}
			h.ctl.SetSubmission(input.Command(draft))
			// Open the composer and return to Command mode at a narrow width.
			h.ctl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
			h.ctl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
			if h.ctl.input.MeasureHeight(30, 30) <= 3 {
				t.Fatal("test command did not wrap")
			}
			h.ctl.HandleKey(keyPress(tea.KeyEnter))
			if len(h.submitted) != 1 || h.submitted[0] != input.Command(draft) {
				t.Fatalf("wrapped submission = %+v", h.submitted)
			}
		})
	}
}

func TestPasteToggleSubmitsCommandLines(t *testing.T) {
	events := make(chan ui.UIEvent, 20)
	m := NewModel(events)
	draft := "north\nlook\n/echo done"
	m.inputCtl.HandlePaste(draft)
	m.inputCtl.HandleKey(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	m.inputCtl.HandleKey(keyPress(tea.KeyEnter))
	for len(events) > 0 {
		if msg, ok := (<-events).(ui.InputSubmittedMsg); ok {
			if msg.Submission != input.Command(draft) {
				t.Fatalf("submission = %+v", msg.Submission)
			}
			return
		}
	}
	t.Fatal("multiline command was not submitted")
}
