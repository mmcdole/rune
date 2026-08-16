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

// numpadKeyInfo describes one physical numpad key. text is the character the
// key types in numeric mode; nav is its semantic fallback when the terminal
// reports the NumLock-off form.
type numpadKeyInfo struct {
	name string
	text string
	nav  rune
}

// numpadCode returns Rune's description of a physical numpad key code.
// Enter has no printable fallback; equal and comma have no public bind name
// but still type normally.
func numpadCode(code rune) (numpadKeyInfo, bool) {
	if code >= tea.KeyKp0 && code <= tea.KeyKp9 {
		digit := string(rune('0') + code - tea.KeyKp0)
		return numpadKeyInfo{name: "numpad" + digit, text: digit}, true
	}
	switch code {
	case tea.KeyKpEnter:
		return numpadKeyInfo{name: "numpad_enter"}, true
	case tea.KeyKpPlus:
		return numpadKeyInfo{name: "numpad_plus", text: "+"}, true
	case tea.KeyKpMinus:
		return numpadKeyInfo{name: "numpad_minus", text: "-"}, true
	case tea.KeyKpMultiply:
		return numpadKeyInfo{name: "numpad_star", text: "*"}, true
	case tea.KeyKpDivide:
		return numpadKeyInfo{name: "numpad_slash", text: "/"}, true
	case tea.KeyKpDecimal:
		return numpadKeyInfo{name: "numpad_dot", text: "."}, true
	case tea.KeyKpEqual:
		return numpadKeyInfo{text: "="}, true
	case tea.KeyKpComma, tea.KeyKpSep:
		return numpadKeyInfo{text: ","}, true
	case tea.KeyKpInsert:
		return numpadKeyInfo{name: "numpad0", nav: tea.KeyInsert}, true
	case tea.KeyKpEnd:
		return numpadKeyInfo{name: "numpad1", nav: tea.KeyEnd}, true
	case tea.KeyKpDown:
		return numpadKeyInfo{name: "numpad2", nav: tea.KeyDown}, true
	case tea.KeyKpPgDown:
		return numpadKeyInfo{name: "numpad3", nav: tea.KeyPgDown}, true
	case tea.KeyKpLeft:
		return numpadKeyInfo{name: "numpad4", nav: tea.KeyLeft}, true
	case tea.KeyKpBegin:
		return numpadKeyInfo{name: "numpad5", nav: tea.KeyBegin}, true
	case tea.KeyKpRight:
		return numpadKeyInfo{name: "numpad6", nav: tea.KeyRight}, true
	case tea.KeyKpHome:
		return numpadKeyInfo{name: "numpad7", nav: tea.KeyHome}, true
	case tea.KeyKpUp:
		return numpadKeyInfo{name: "numpad8", nav: tea.KeyUp}, true
	case tea.KeyKpPgUp:
		return numpadKeyInfo{name: "numpad9", nav: tea.KeyPgUp}, true
	case tea.KeyKpDelete:
		return numpadKeyInfo{name: "numpad_dot", nav: tea.KeyDelete}, true
	default:
		return numpadKeyInfo{}, false
	}
}

// numpadKey recognizes both the direct KeyKp* form used by SS3/kitty input
// and the BaseCode form used by win32 input.
func numpadKey(msg tea.KeyPressMsg) (numpadKeyInfo, bool) {
	if info, ok := numpadCode(msg.BaseCode); ok {
		return info, true
	}
	return numpadCode(msg.Code)
}

// numpadNavigation recognizes the actual NumLock-off code carried by an
// event. Binding identity is BaseCode-first, but semantic fallback follows
// Code so an alternate keyboard-layout code cannot change the key's action.
func numpadNavigation(msg tea.KeyPressMsg) (numpadKeyInfo, bool) {
	info, ok := numpadCode(msg.Code)
	return info, ok && info.nav != 0
}

// navigationFallback converts a NumLock-off physical keypad event into the
// ordinary navigation event engraved on that key. It is deliberately pure:
// input modes decide when this fallback takes precedence over Lua binds.
func (info numpadKeyInfo) navigationFallback(msg tea.KeyPressMsg) tea.KeyPressMsg {
	if info.nav == 0 {
		return msg
	}
	msg.Code = info.nav
	msg.BaseCode = 0
	return msg
}

func normalizeNumpadText(msg tea.KeyPressMsg) tea.KeyPressMsg {
	info, ok := numpadKey(msg)
	if !ok || info.text == "" || info.nav != 0 {
		return msg
	}
	if msg.Mod&(keyModifiers&^tea.ModShift) != 0 {
		// Win32 input supplies the ordinary keypad character even for
		// modified chords. Remove that synthetic text so the same chord
		// routes like SS3 and kitty input, while preserving real AltGr text.
		if msg.Text == info.text {
			msg.Text = ""
		}
		return msg
	}
	if msg.Text == "" {
		msg.Text = info.text
	}
	return msg
}

// keyToString returns Rune's canonical name at the Go/Lua bind boundary.
func keyToString(msg tea.KeyPressMsg) string {
	if info, ok := numpadKey(msg); ok {
		if info.name == "" {
			return ""
		}
		// Reuse Bubble Tea's modifier spelling and ordering while replacing its
		// deliberately collapsed keypad name with Rune's physical-key name.
		msg.Code = tea.KeyExtended
		msg.BaseCode = 0
		msg.Text = info.name
	}
	return msg.Keystroke()
}
