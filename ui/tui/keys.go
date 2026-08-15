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

// numpadCode returns Rune's bind name and printable fallback for a physical
// numpad key code. Enter has no printable fallback; equal and comma have no
// public bind name but still type normally.
func numpadCode(code rune) (name, text string, ok bool) {
	if code >= tea.KeyKp0 && code <= tea.KeyKp9 {
		digit := string(rune('0') + code - tea.KeyKp0)
		return "numpad" + digit, digit, true
	}
	switch code {
	case tea.KeyKpEnter:
		return "numpad_enter", "", true
	case tea.KeyKpPlus:
		return "numpad_plus", "+", true
	case tea.KeyKpMinus:
		return "numpad_minus", "-", true
	case tea.KeyKpMultiply:
		return "numpad_star", "*", true
	case tea.KeyKpDivide:
		return "numpad_slash", "/", true
	case tea.KeyKpDecimal:
		return "numpad_dot", ".", true
	case tea.KeyKpEqual:
		return "", "=", true
	case tea.KeyKpComma, tea.KeyKpSep:
		return "", ",", true
	default:
		return "", "", false
	}
}

// numpadKey recognizes both the direct KeyKp* form used by SS3/kitty input
// and the BaseCode form used by win32 input.
func numpadKey(msg tea.KeyPressMsg) (name, text string, ok bool) {
	if name, text, ok = numpadCode(msg.BaseCode); ok {
		return name, text, true
	}
	return numpadCode(msg.Code)
}

func normalizeNumpadText(msg tea.KeyPressMsg) tea.KeyPressMsg {
	_, text, ok := numpadKey(msg)
	if !ok || text == "" {
		return msg
	}
	if msg.Mod&(keyModifiers&^tea.ModShift) != 0 {
		// Win32 input supplies the ordinary keypad character even for
		// modified chords. Remove that synthetic text so the same chord
		// routes like SS3 and kitty input, while preserving real AltGr text.
		if msg.Text == text {
			msg.Text = ""
		}
		return msg
	}
	if msg.Text == "" {
		msg.Text = text
	}
	return msg
}

// keyToString returns Rune's canonical name at the Go/Lua bind boundary.
func keyToString(msg tea.KeyPressMsg) string {
	if name, _, ok := numpadKey(msg); ok {
		if name == "" {
			return ""
		}
		// Reuse Bubble Tea's modifier spelling and ordering while replacing its
		// deliberately collapsed keypad name with Rune's physical-key name.
		msg.Code = tea.KeyExtended
		msg.BaseCode = 0
		msg.Text = name
	}
	return msg.Keystroke()
}
