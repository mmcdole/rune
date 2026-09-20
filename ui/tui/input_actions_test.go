package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

func TestDefaultNewlineBindingsInBothInputModes(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{
		{Code: 'j', Mod: tea.ModCtrl}, {Code: tea.KeyEnter, Mod: tea.ModShift},
		{Code: tea.KeyEnter, Mod: tea.ModCtrl}, {Code: tea.KeyKpEnter, Mod: tea.ModShift},
	} {
		h := newControllerHarness()
		h.ctl.SetText("hello")
		h.ctl.HandleKey(msg)
		h.ctl.HandleKey(msg)
		if h.ctl.input.Value() != "hello\n\n" || len(h.submitted) != 0 {
			t.Fatalf("%v: draft %q, submissions %+v", msg, h.ctl.input.Value(), h.submitted)
		}
	}
}

func TestInputBindingsChangeRoutingAndHints(t *testing.T) {
	m := newBareModel(t)
	m.input.SetSize(120, 0)
	keys := input.Bindings{
		"ctrl+s":      {Action: "input.submit", Enabled: true},
		"shift+enter": {Action: "input.newline", Enabled: true, Order: 1},
		"ctrl+j":      {Action: "input.newline", Enabled: true, Order: 2},
		"ctrl+t":      {Action: "input.toggle_mode", Enabled: true},
	}
	m.Update(ui.UpdateBindsMsg(keys))
	m.inputCtl.HandlePaste("first\nsecond")
	m.input.SetSize(120, 5)
	var labels string
	for _, label := range m.input.Labels() {
		labels += label.Text
	}
	for _, want := range []string{"Ctrl+S send", "Shift+Enter newline", "Ctrl+T command"} {
		if !strings.Contains(labels, want) {
			t.Fatalf("missing %s: %s", want, labels)
		}
	}
	if strings.Contains(labels, "Ctrl+J") || strings.Contains(labels, "Alt+Enter") {
		t.Fatalf("unexpected secondary hint: %s", labels)
	}
	h := newControllerHarness()
	h.ctl.input.SetBindings(keys)
	h.ctl.HandlePaste("first\nsecond")
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 0 {
		t.Fatal("old submit binding remained active")
	}
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if h.ctl.input.SubmissionMode() != input.ModeCommand {
		t.Fatal("toggle did not change mode")
	}
	h.accept = false
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if h.ctl.input.Value() != "first\nsecond" || h.submitted[0].Mode != input.ModeCommand {
		t.Fatal("rejected submit lost draft or mode")
	}
}

func TestInputActionsRespectModalPicker(t *testing.T) {
	h := newControllerHarness()
	h.ctl.input.SetBindings(input.Bindings{
		"f1": {Action: "input.submit", Enabled: true},
		"f2": {Action: "input.newline", Enabled: true},
		"f3": {Action: "input.toggle_mode", Enabled: true},
	})
	h.ctl.SetText("draft")
	h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "modal"})
	for _, code := range []rune{tea.KeyF1, tea.KeyF2, tea.KeyF3} {
		h.ctl.HandleKey(keyPress(code))
	}
	if h.ctl.input.Value() != "draft" || len(h.submitted) != 0 || h.ctl.input.SubmissionMode() != input.ModeCommand {
		t.Fatal("input action escaped modal picker")
	}
}

func TestInlinePickerEnterSelectsWithoutReboundSubmit(t *testing.T) {
	h := newControllerHarness()
	h.ctl.input.SetBindings(input.Bindings{"ctrl+s": {Action: "input.submit", Enabled: true}})
	h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "inline", Inline: true})
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if h.ctl.input.PickerActive() || len(h.submitted) != 0 {
		t.Fatal("Enter must only settle the inline picker when submit is rebound")
	}
	h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "inline2", Inline: true})
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if h.ctl.input.PickerActive() || len(h.submitted) != 1 {
		t.Fatal("configured submit must select and submit")
	}
}

func TestBindingReplacementRemovalAndDisabledHints(t *testing.T) {
	m := newBareModel(t)
	bindings := input.DefaultBindings()
	bindings["enter"] = input.Binding{Enabled: true} // Lua callback
	m.Update(ui.UpdateBindsMsg(bindings))
	m.inputCtl.SetText("draft")
	m.inputCtl.HandleKey(keyPress(tea.KeyEnter))
	if m.input.Value() != "draft" || m.input.Bindings().Hint("submit") != "" {
		t.Fatal("callback replacement retained submit behavior or hint")
	}
	bindings = input.DefaultBindings()
	binding := bindings["ctrl+j"]
	binding.Enabled = false
	bindings["ctrl+j"] = binding
	m.Update(ui.UpdateBindsMsg(bindings))
	m.inputCtl.HandleKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if m.input.Value() != "draft" || m.input.Bindings().Hint("newline") != "Shift+Enter" {
		t.Fatal("disabled binding acted or remained the hint")
	}
	m.Update(ui.UpdateBindsMsg{})
	m.inputCtl.HandleKey(keyPress(tea.KeyEnter))
	if m.input.Value() != "draft" || m.input.Bindings().Hint("submit") != "" {
		t.Fatal("empty snapshot resurrected defaults")
	}
}

func TestNamedActionRespectsTypingAndPhysicalKeypad(t *testing.T) {
	h := newControllerHarness()
	bindings := input.DefaultBindings()
	bindings["j"] = input.Binding{Action: "input.submit", Enabled: true}
	bindings["numpad_enter"] = input.Binding{Action: "input.newline", Enabled: true}
	h.ctl.input.SetBindings(bindings)
	h.ctl.SetText("draft")
	h.ctl.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if h.ctl.input.Value() != "draftj" || len(h.submitted) != 0 {
		t.Fatal("named binding swallowed typed text")
	}
	h.ctl.HandleKey(keyPress(tea.KeyKpEnter))
	if h.ctl.input.Value() != "draftj\n" || len(h.submitted) != 0 {
		t.Fatal("physical keypad action lost to Enter default")
	}
}

func TestReboundCancelAcrossInputContexts(t *testing.T) {
	for _, context := range []string{"normal", "draft_editor", "inline", "modal", "search"} {
		t.Run(context, func(t *testing.T) {
			h := newControllerHarness()
			bindings := input.DefaultBindings()
			delete(bindings, "esc")
			bindings["ctrl+g"] = input.Binding{Action: "input.cancel", Enabled: true}
			h.ctl.input.SetBindings(bindings)
			h.ctl.SetText("draft")
			switch context {
			case "draft_editor":
				h.ctl.SetText("first\nsecond")
			case "inline", "modal":
				h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "test", Inline: context == "inline"})
			case "search":
				h.ctl.ShowSearch(ui.ShowSearchMsg{})
			}
			mode, draft := h.ctl.mode(), h.ctl.input.Value()
			h.ctl.HandleKey(keyPress(tea.KeyEsc))
			if h.ctl.mode() != mode || h.ctl.input.Value() != draft {
				t.Fatal("unbound Escape still cancels")
			}
			h.ctl.HandleKey(ctrlPress('g'))
			if context == "draft_editor" {
				if h.ctl.input.Value() != draft {
					t.Fatal("first cancel discarded draft editor")
				}
				h.ctl.HandleKey(ctrlPress('g'))
			}
			if h.ctl.mode() != modeNormal {
				t.Fatal("cancel did not close context")
			}
			if context == "normal" || context == "draft_editor" {
				if h.ctl.input.Value() != "" {
					t.Fatal("cancel did not clear draft")
				}
			} else if h.ctl.input.Value() != draft {
				t.Fatal("overlay cancel changed draft")
			}
			if context == "inline" || context == "modal" {
				if events := h.pickerSelects(); len(events) != 1 || events[0].Accepted {
					t.Fatalf("picker completion: %+v", events)
				}
			}
			if context == "search" && h.cancels != 1 {
				t.Fatal("search view was not restored")
			}
		})
	}
}

func TestExternalEditorAndCancelHintsFollowActions(t *testing.T) {
	m := newBareModel(t)
	m.input.SetSize(140, 0)
	bindings := input.DefaultBindings()
	delete(bindings, "esc")
	delete(bindings, "ctrl+e")
	bindings["ctrl+g"] = input.Binding{Action: "input.cancel", Enabled: true}
	bindings["f2"] = input.Binding{Action: "input.open_editor", Enabled: true}
	m.Update(ui.UpdateBindsMsg(bindings))
	m.inputCtl.HandlePaste("first\nsecond")
	m.input.SetSize(140, 5)
	labels := func() string {
		var s string
		for _, label := range m.input.Labels() {
			s += label.Text
		}
		return s
	}
	if s := labels(); !strings.Contains(s, "Ctrl+G×2 discard") || !strings.Contains(s, "F2 editor") || strings.Contains(s, "Esc") || strings.Contains(s, "Ctrl+E") {
		t.Fatalf("hints: %s", s)
	}
	m.inputCtl.HandleKey(ctrlPress('g'))
	if s := labels(); !strings.Contains(s, "Ctrl+G again to discard") {
		t.Fatalf("confirmation: %s", s)
	}
	// Any unrelated key dismisses confirmation; the next cancel only arms it.
	m.inputCtl.HandleKey(keyPress(tea.KeyLeft))
	m.inputCtl.HandleKey(ctrlPress('g'))
	if m.input.Value() == "" {
		t.Fatal("intervening key did not dismiss confirmation")
	}
	bindings["ctrl+g"] = input.Binding{Action: "input.cancel", Enabled: false}
	bindings["f2"] = input.Binding{Enabled: true} // replacement callback
	m.Update(ui.UpdateBindsMsg(bindings))
	if s := labels(); strings.Contains(s, "discard") || strings.Contains(s, "editor") {
		t.Fatalf("inactive hints: %s", s)
	}
	m.inputCtl.HandleKey(ctrlPress('g'))
	if m.input.Value() == "" {
		t.Fatal("disabled cancel discarded draft")
	}
}

func TestReboundExternalEditorUsesDraftAndRespectsOverlays(t *testing.T) {
	for _, context := range []string{"normal", "draft_editor", "inline", "modal", "search"} {
		t.Run(context, func(t *testing.T) {
			h := newControllerHarness()
			bindings := input.DefaultBindings()
			delete(bindings, "ctrl+e")
			bindings["f2"] = input.Binding{Action: "input.open_editor", Enabled: true}
			h.ctl.input.SetBindings(bindings)
			h.ctl.SetText("draft")
			switch context {
			case "draft_editor":
				h.ctl.SetText("first\nsecond")
			case "inline", "modal":
				h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "test", Inline: context == "inline"})
			case "search":
				h.ctl.ShowSearch(ui.ShowSearchMsg{})
			}
			h.events = nil
			h.ctl.HandleKey(ctrlPress('e'))
			h.ctl.HandleKey(keyPress(tea.KeyF2))
			count := 0
			for _, event := range h.events {
				if editor, ok := event.(ui.OpenEditorMsg); ok {
					count++
					if editor.Text != h.ctl.input.Value() {
						t.Fatal("editor got dirty draft")
					}
				}
			}
			want := 1
			if context == "modal" || context == "search" {
				want = 0
			}
			if count != want {
				t.Fatalf("editor requests %d, want %d", count, want)
			}
		})
	}
}

func TestSearchCancelHintUsesBindingSnapshot(t *testing.T) {
	m := newBareModel(t)
	m.input.SetSize(100, 0)
	m.Update(ui.UpdateBindsMsg{"ctrl+g": {Action: "input.cancel", Enabled: true}})
	m.inputCtl.ShowSearch(ui.ShowSearchMsg{})
	m.input.SetSize(100, 8)
	if view := m.input.View(); !strings.Contains(view, "Ctrl+G cancel") || strings.Contains(view, "Esc cancel") {
		t.Fatalf("search hint: %s", view)
	}
	m.Update(ui.UpdateBindsMsg{})
	m.input.SetSize(100, 8)
	if view := m.input.View(); strings.Contains(view, "cancel") {
		t.Fatalf("unbound search hint: %s", view)
	}
}
