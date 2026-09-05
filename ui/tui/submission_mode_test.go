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

func TestRunAsCommandIsOneOffAndRetainsRejectedDraft(t *testing.T) {
	h := newControllerHarness()
	h.ctl.HandlePaste("/lua -- comment\nrune.echo('hello')")
	h.ctl.input.SetCursor(5)
	h.accept = false
	h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if len(h.submitted) != 1 || h.submitted[0].Mode != input.ModeCommand {
		t.Fatalf("override = %+v", h.submitted)
	}
	if h.ctl.input.Value() != h.submitted[0].Text || h.ctl.input.Position() != 5 || h.ctl.input.SubmissionMode() != input.ModeVerbatim {
		t.Fatal("rejected override changed the draft")
	}
	h.accept = true
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 2 || h.submitted[1].Mode != input.ModeVerbatim {
		t.Fatal("override changed subsequent Enter interpretation")
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
	m.inputCtl.HandlePaste("north\nlook")
	m.inputCtl.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if m.input.Value() != "north\nlook" || m.input.SubmissionMode() != input.ModeVerbatim {
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
	m.inputCtl.HandleKey(keyPress(tea.KeyEnter))
	msg, ok := (<-events).(ui.InputSubmittedMsg)
	if !ok || msg.Submission != input.Verbatim("north\nlook") {
		t.Fatalf("verbatim retry = %#v", msg)
	}
}

func TestOversizedMultilineCommandCannotBypassDraftLimits(t *testing.T) {
	m := newBareModel(t)
	for _, draft := range []string{"/lua " + strings.Repeat("\n", maxSubmissionLines), "/lua " + strings.Repeat("x", maxSubmissionBytes)} {
		if m.submit(ui.InputSubmittedMsg{Submission: input.Command(draft)}) {
			t.Fatal("oversized command accepted")
		}
	}
}

func TestComposerEditorHintTracksBindingUpdates(t *testing.T) {
	m := newBareModel(t)
	m.input.SetSize(100, 0)
	m.inputCtl.HandlePaste("first\nsecond")
	for _, available := range []bool{false, true, false} {
		m.Update(ui.UpdateBindsMsg{"ctrl+e": available})
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
