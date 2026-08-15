package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKeyToStringUsesCanonicalV2Names(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{name: "escape", msg: tea.KeyPressMsg{Code: tea.KeyEsc}, want: "esc"},
		{name: "page up", msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: "pgup"},
		{name: "keypad enter", msg: tea.KeyPressMsg{Code: tea.KeyKpEnter}, want: "enter"},
		{name: "modified navigation", msg: tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}, want: "ctrl+home"},
		{name: "extended function key", msg: tea.KeyPressMsg{Code: tea.KeyF20}, want: "f20"},
		{name: "highest function key", msg: tea.KeyPressMsg{Code: tea.KeyF63}, want: "f63"},
		{name: "shifted printable key", msg: tea.KeyPressMsg{Code: 'a', Text: "A", Mod: tea.ModShift}, want: "shift+a"},
		{name: "modified printable key", msg: tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt}, want: "alt+x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyToString(tt.msg); got != tt.want {
				t.Fatalf("keyToString(%#v) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}
