package tui

import tea "charm.land/bubbletea/v2"

const keyModifiers = tea.ModShift | tea.ModAlt | tea.ModCtrl |
	tea.ModMeta | tea.ModHyper | tea.ModSuper

// matchesKey compares the physical key and actionable modifiers while
// ignoring lock state such as Caps Lock and Num Lock.
func matchesKey(msg tea.KeyPressMsg, code rune, modifiers tea.KeyMod) bool {
	return msg.Code == code && msg.Mod&keyModifiers == modifiers
}

func isEnterKey(msg tea.KeyPressMsg) bool {
	return msg.Code == tea.KeyEnter || msg.Code == tea.KeyKpEnter
}

func matchesEnterKey(msg tea.KeyPressMsg, modifiers tea.KeyMod) bool {
	return isEnterKey(msg) && msg.Mod&keyModifiers == modifiers
}

// keyToString returns Bubble Tea's canonical name at the Go/Lua bind boundary.
func keyToString(msg tea.KeyPressMsg) string {
	return msg.Keystroke()
}
