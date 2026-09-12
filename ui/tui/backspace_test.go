package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

// Regression for #134: a terminal can report Shift+Backspace separately.
func TestShiftBackspaceDeletesInEveryInputMode(t *testing.T) {
	for _, mode := range []string{"normal", "compose", "inline", "modal", "search", "selected", "selected compose"} {
		for _, tc := range []struct {
			name   string
			text   string
			cursor int
			want   string
		}{
			{"end", "HELLOÉ", 6, "HELLO"},
			{"middle", "HELLOÉ", 3, "HELOÉ"},
			{"start", "HELLOÉ", 0, "HELLOÉ"},
			{"empty", "", 0, ""},
		} {
			// Overlay queries append and delete at the end; they have no cursor.
			if (mode == "modal" || mode == "search") && (tc.name == "middle" || tc.name == "start") {
				continue
			}
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				h := newControllerHarness()
				in := h.ctl.input
				in.SetValue(tc.text)
				in.SetCursor(tc.cursor)
				value := in.Value
				want := tc.want
				switch mode {
				case "compose", "selected compose":
					in.BeginCompose(tc.text, tc.cursor)
				case "inline", "modal":
					h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, Inline: mode == "inline"})
					if mode == "modal" {
						in.Picker().Filter(tc.text)
						value = in.Picker().Query
					}
				case "search":
					h.ctl.ShowSearch(ui.ShowSearchMsg{Query: tc.text})
					value = in.Search().Query
				}
				if mode == "selected" || mode == "selected compose" {
					in.SelectAll()
					want = ""
				}
				h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModShift})
				if got := value(); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestShiftBackspaceBindingPrecedence(t *testing.T) {
	for _, mode := range []string{"normal", "inline", "compose"} {
		t.Run(mode, func(t *testing.T) {
			h := newControllerHarness()
			h.bound["shift+backspace"] = true
			h.ctl.input.SetValue("HELLO")
			h.ctl.input.CursorEnd()
			if mode == "inline" {
				h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, Inline: true})
			}
			if mode == "compose" {
				h.ctl.input.BeginCompose("HELLO", 5)
			}
			h.events = nil
			h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModShift})
			var binds int
			for _, ev := range h.events {
				if key, ok := ev.(ui.ExecuteBindMsg); ok && string(key) == "shift+backspace" {
					binds++
				}
			}
			want, wantBinds := "HELLO", 1
			if mode == "compose" {
				want, wantBinds = "HELL", 0
			}
			if got := h.ctl.input.Value(); got != want || binds != wantBinds {
				t.Fatalf("text=%q binds=%d, want text=%q binds=%d", got, binds, want, wantBinds)
			}
		})
	}
}
