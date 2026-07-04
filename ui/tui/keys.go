package tui

import tea "github.com/charmbracelet/bubbletea"

// keyNames maps Bubble Tea key types to string names for Lua bindings.
var keyNames = map[tea.KeyType]string{
	tea.KeyCtrlA:      "ctrl+a",
	tea.KeyCtrlB:      "ctrl+b",
	tea.KeyCtrlC:      "ctrl+c",
	tea.KeyCtrlD:      "ctrl+d",
	tea.KeyCtrlE:      "ctrl+e",
	tea.KeyCtrlF:      "ctrl+f",
	tea.KeyCtrlG:      "ctrl+g",
	tea.KeyCtrlH:      "ctrl+h",
	tea.KeyCtrlI:      "tab", // Same as KeyTab
	tea.KeyShiftTab:   "shift+tab",
	tea.KeyCtrlJ:      "ctrl+j",
	tea.KeyCtrlK:      "ctrl+k",
	tea.KeyCtrlL:      "ctrl+l",
	tea.KeyCtrlM:      "ctrl+m",
	tea.KeyCtrlN:      "ctrl+n",
	tea.KeyCtrlO:      "ctrl+o",
	tea.KeyCtrlP:      "ctrl+p",
	tea.KeyCtrlQ:      "ctrl+q",
	tea.KeyCtrlR:      "ctrl+r",
	tea.KeyCtrlS:      "ctrl+s",
	tea.KeyCtrlT:      "ctrl+t",
	tea.KeyCtrlU:      "ctrl+u",
	tea.KeyCtrlV:      "ctrl+v",
	tea.KeyCtrlW:      "ctrl+w",
	tea.KeyCtrlX:      "ctrl+x",
	tea.KeyCtrlY:      "ctrl+y",
	tea.KeyCtrlZ:      "ctrl+z",
	tea.KeyF1:         "f1",
	tea.KeyF2:         "f2",
	tea.KeyF3:         "f3",
	tea.KeyF4:         "f4",
	tea.KeyF5:         "f5",
	tea.KeyF6:         "f6",
	tea.KeyF7:         "f7",
	tea.KeyF8:         "f8",
	tea.KeyF9:         "f9",
	tea.KeyF10:        "f10",
	tea.KeyF11:        "f11",
	tea.KeyF12:        "f12",
	tea.KeyUp:         "up",
	tea.KeyDown:       "down",
	tea.KeyLeft:       "left",
	tea.KeyRight:      "right",
	tea.KeyCtrlUp:     "ctrl+up",
	tea.KeyCtrlDown:   "ctrl+down",
	tea.KeyCtrlLeft:   "ctrl+left",
	tea.KeyCtrlRight:  "ctrl+right",
	tea.KeyShiftUp:    "shift+up",
	tea.KeyShiftDown:  "shift+down",
	tea.KeyShiftLeft:  "shift+left",
	tea.KeyShiftRight: "shift+right",
	tea.KeyEsc:        "escape",
	tea.KeyBackspace:  "backspace",
	tea.KeyDelete:     "delete",
	tea.KeyInsert:     "insert",
	tea.KeyPgUp:       "pageup",
	tea.KeyPgDown:     "pagedown",
	tea.KeyCtrlPgUp:   "ctrl+pageup",
	tea.KeyCtrlPgDown: "ctrl+pagedown",
	tea.KeyHome:       "home",
	tea.KeyEnd:        "end",
	tea.KeyCtrlHome:   "ctrl+home",
	tea.KeyCtrlEnd:    "ctrl+end",
	tea.KeyShiftHome:  "shift+home",
	tea.KeyShiftEnd:   "shift+end",
}

// keyToString converts a key press to the name Lua binds use. The alt
// modifier arrives as a flag on the base key (bubbletea reports alt+left
// as KeyLeft with Alt set), so it is prefixed here; ctrl- and shift-
// modified keys are distinct KeyTypes and come from the table.
func keyToString(msg tea.KeyMsg) string {
	var base string
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		base = string(msg.Runes)
	} else {
		base = keyNames[msg.Type]
	}
	if base == "" {
		return ""
	}
	if msg.Alt {
		return "alt+" + base
	}
	return base
}
