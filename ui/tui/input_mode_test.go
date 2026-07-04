package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// controllerHarness drives an inputController directly, recording
// outbound events and submitted lines.
type controllerHarness struct {
	ctl       *inputController
	events    []ui.UIEvent
	submitted []string
}

func newControllerHarness() *controllerHarness {
	h := &controllerHarness{}
	input := widget.NewInput(style.DefaultStyles())
	h.ctl = newInputController(
		input,
		func(ev ui.UIEvent) { h.events = append(h.events, ev) },
		func(line string) { h.submitted = append(h.submitted, line) },
		func(string) bool { return false },
		func(tea.KeyType) bool { return false },
	)
	return h
}

func (h *controllerHarness) pickerSelects() []ui.PickerSelectMsg {
	var out []ui.PickerSelectMsg
	for _, ev := range h.events {
		if sel, ok := ev.(ui.PickerSelectMsg); ok {
			out = append(out, sel)
		}
	}
	return out
}

var pickerTestItems = []ui.PickerItem{
	{Text: "/connect", Value: "/connect"},
	{Text: "/disconnect", Value: "/disconnect"},
}

// TestPickerCallbackSettledOnEveryExit verifies the controller's core
// invariant: every path out of a picker mode sends exactly one
// PickerSelectMsg (accepted or cancelled) and resets the mode - even
// the paths that used to strand the callback, like closing a picker
// with nothing selected.
func TestPickerCallbackSettledOnEveryExit(t *testing.T) {
	cases := []struct {
		name     string
		inline   bool
		setup    func(h *controllerHarness)
		key      tea.KeyMsg
		accepted bool
		value    string
	}{
		{
			name:     "modal escape cancels",
			key:      tea.KeyMsg{Type: tea.KeyEsc},
			accepted: false,
		},
		{
			name:     "modal ctrl+c cancels",
			key:      tea.KeyMsg{Type: tea.KeyCtrlC},
			accepted: false,
		},
		{
			name:     "modal enter accepts selection",
			key:      tea.KeyMsg{Type: tea.KeyEnter},
			accepted: true,
			value:    "/connect",
		},
		{
			name: "modal enter with no match cancels",
			setup: func(h *controllerHarness) {
				h.ctl.input.PickerFilter("zzz")
			},
			key:      tea.KeyMsg{Type: tea.KeyEnter},
			accepted: false,
		},
		{
			name:     "inline escape cancels",
			inline:   true,
			key:      tea.KeyMsg{Type: tea.KeyEsc},
			accepted: false,
		},
		{
			name:     "inline tab accepts selection",
			inline:   true,
			key:      tea.KeyMsg{Type: tea.KeyTab},
			accepted: true,
			value:    "/connect",
		},
		{
			name:   "inline tab with no match cancels",
			inline: true,
			setup: func(h *controllerHarness) {
				h.ctl.SetText("zzz")
			},
			key:      tea.KeyMsg{Type: tea.KeyTab},
			accepted: false,
		},
		{
			name:     "inline enter accepts and submits",
			inline:   true,
			key:      tea.KeyMsg{Type: tea.KeyEnter},
			accepted: true,
			value:    "/connect",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newControllerHarness()
			h.ctl.ShowPicker(ui.ShowPickerMsg{
				Items:      pickerTestItems,
				CallbackID: "cb",
				Inline:     tc.inline,
			})
			if tc.setup != nil {
				tc.setup(h)
			}

			h.ctl.HandleKey(tc.key)

			selects := h.pickerSelects()
			if len(selects) != 1 {
				t.Fatalf("expected exactly one PickerSelectMsg, got %d: %v", len(selects), selects)
			}
			sel := selects[0]
			if sel.CallbackID != "cb" || sel.Accepted != tc.accepted || sel.Value != tc.value {
				t.Fatalf("got %+v, want {CallbackID: cb, Accepted: %v, Value: %q}",
					sel, tc.accepted, tc.value)
			}
			if h.ctl.mode != ModeNormal {
				t.Fatalf("expected ModeNormal after exit, got %v", h.ctl.mode)
			}
		})
	}
}

// TestSubmitReportsClearedInput verifies Enter delivers the line and
// reports the cleared input, so the session's tracked input
// (rune.input.get) cannot go stale after a submit.
func TestSubmitReportsClearedInput(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetText("look north")
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if len(h.submitted) != 1 || h.submitted[0] != "look north" {
		t.Fatalf("expected submit of %q, got %v", "look north", h.submitted)
	}
	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("expected input cleared after submit, got %q", got)
	}
	if len(h.events) != 1 {
		t.Fatalf("expected one event after submit, got %v", h.events)
	}
	ic, ok := h.events[0].(ui.InputChangedMsg)
	if !ok || ic.Text != "" || ic.Cursor != 0 {
		t.Fatalf("expected InputChangedMsg{Text: \"\", Cursor: 0}, got %+v", h.events[0])
	}
}

// TestInlineTabReportsCompletedInput verifies a Tab completion reports
// the new input text to the session before the selection callback
// fires, so the callback observes fresh input state.
func TestInlineTabReportsCompletedInput(t *testing.T) {
	h := newControllerHarness()
	h.ctl.ShowPicker(ui.ShowPickerMsg{
		Items:      pickerTestItems,
		CallbackID: "cb",
		Inline:     true,
	})
	h.ctl.SetText("/con")
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyTab})

	changedAt, selectAt := -1, -1
	for i, ev := range h.events {
		switch ev.(type) {
		case ui.InputChangedMsg:
			if changedAt == -1 {
				changedAt = i
			}
		case ui.PickerSelectMsg:
			selectAt = i
		}
	}
	if changedAt == -1 {
		t.Fatal("tab completion did not report the changed input")
	}
	if selectAt == -1 {
		t.Fatal("tab completion did not settle the picker callback")
	}
	if changedAt > selectAt {
		t.Fatal("InputChangedMsg must precede PickerSelectMsg so the callback sees fresh input")
	}
	ic := h.events[changedAt].(ui.InputChangedMsg)
	if ic.Text != "/connect " {
		t.Fatalf("expected completed input %q, got %q", "/connect ", ic.Text)
	}
	if got := h.ctl.input.Value(); got != "/connect " {
		t.Fatalf("expected input %q after completion, got %q", "/connect ", got)
	}
}
