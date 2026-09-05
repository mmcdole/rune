package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func TestKeyToStringUsesCanonicalV2Names(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{name: "escape", msg: tea.KeyPressMsg{Code: tea.KeyEsc}, want: "esc"},
		{name: "page up", msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: "pgup"},
		{name: "number row", msg: tea.KeyPressMsg{Code: '8', Text: "8"}, want: "8"},
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

func TestKeyToStringUsesCanonicalNumpadNames(t *testing.T) {
	tests := []struct {
		code rune
		want string
	}{
		{tea.KeyKp0, "numpad0"},
		{tea.KeyKp1, "numpad1"},
		{tea.KeyKp2, "numpad2"},
		{tea.KeyKp3, "numpad3"},
		{tea.KeyKp4, "numpad4"},
		{tea.KeyKp5, "numpad5"},
		{tea.KeyKp6, "numpad6"},
		{tea.KeyKp7, "numpad7"},
		{tea.KeyKp8, "numpad8"},
		{tea.KeyKp9, "numpad9"},
		{tea.KeyKpDecimal, "numpad_dot"},
		{tea.KeyKpDivide, "numpad_slash"},
		{tea.KeyKpMultiply, "numpad_star"},
		{tea.KeyKpMinus, "numpad_minus"},
		{tea.KeyKpPlus, "numpad_plus"},
		{tea.KeyKpEnter, "numpad_enter"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := keyToString(tea.KeyPressMsg{Code: tt.code}); got != tt.want {
				t.Fatalf("keyToString(Code=%v) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestNumpadNameNormalizesInputEncodingsAndModifiers(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{name: "DECKPAM", msg: tea.KeyPressMsg{Code: tea.KeyKp8}, want: "numpad8"},
		{name: "kitty", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Text: "8"}, want: "numpad8"},
		{name: "win32", msg: tea.KeyPressMsg{Code: '8', BaseCode: tea.KeyKp8, Text: "8"}, want: "numpad8"},
		{name: "modifier", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModCtrl}, want: "ctrl+numpad8"},
		{name: "NumLock", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Text: "8", Mod: tea.ModNumLock}, want: "numpad8"},
		{name: "Enter distinct from minus", msg: tea.KeyPressMsg{Code: tea.KeyKpEnter}, want: "numpad_enter"},
		{name: "minus distinct from Enter", msg: tea.KeyPressMsg{Code: tea.KeyKpMinus}, want: "numpad_minus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyToString(tt.msg); got != tt.want {
				t.Fatalf("keyToString(%#v) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestNormalizeNumpadTextFillsOnlyPrintableUnmodifiedKeys(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{name: "digit", msg: tea.KeyPressMsg{Code: tea.KeyKp8}, want: "8"},
		{name: "operator", msg: tea.KeyPressMsg{Code: tea.KeyKpMinus}, want: "-"},
		{name: "equal", msg: tea.KeyPressMsg{Code: tea.KeyKpEqual}, want: "="},
		{name: "comma", msg: tea.KeyPressMsg{Code: tea.KeyKpComma}, want: ","},
		{name: "separator", msg: tea.KeyPressMsg{Code: tea.KeyKpSep}, want: ","},
		{name: "Shift", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModShift}, want: "8"},
		{name: "NumLock", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModNumLock}, want: "8"},
		{name: "existing text", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Text: "existing"}, want: "existing"},
		{name: "Enter", msg: tea.KeyPressMsg{Code: tea.KeyKpEnter}, want: ""},
		{name: "Ctrl chord", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModCtrl}, want: ""},
		{name: "Alt chord", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModAlt}, want: ""},
		{name: "win32 Ctrl chord", msg: tea.KeyPressMsg{Code: '8', BaseCode: tea.KeyKp8, Text: "8", Mod: tea.ModCtrl}, want: ""},
		{name: "AltGr text", msg: tea.KeyPressMsg{Code: '@', BaseCode: tea.KeyKp8, Text: "@", Mod: tea.ModCtrl | tea.ModAlt}, want: "@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeNumpadText(tt.msg).Text; got != tt.want {
				t.Fatalf("normalized Text = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestKpNavigationKeysShareDigitBindNames covers the NumLock-off keypad
// reported by the kitty keyboard protocol: the physical key keeps its
// numpad bind name but must not type a digit.
func TestKpNavigationKeysShareDigitBindNames(t *testing.T) {
	tests := []struct {
		code rune
		name string
		nav  rune
	}{
		{tea.KeyKpInsert, "numpad0", tea.KeyInsert},
		{tea.KeyKpEnd, "numpad1", tea.KeyEnd},
		{tea.KeyKpDown, "numpad2", tea.KeyDown},
		{tea.KeyKpPgDown, "numpad3", tea.KeyPgDown},
		{tea.KeyKpLeft, "numpad4", tea.KeyLeft},
		{tea.KeyKpBegin, "numpad5", tea.KeyBegin},
		{tea.KeyKpRight, "numpad6", tea.KeyRight},
		{tea.KeyKpHome, "numpad7", tea.KeyHome},
		{tea.KeyKpUp, "numpad8", tea.KeyUp},
		{tea.KeyKpPgUp, "numpad9", tea.KeyPgUp},
		{tea.KeyKpDelete, "numpad_dot", tea.KeyDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tea.KeyPressMsg{Code: tt.code, Mod: tea.ModCtrl}
			if got := keyToString(msg); got != "ctrl+"+tt.name {
				t.Fatalf("keyToString(Code=%v) = %q, want %q", tt.code, got, "ctrl+"+tt.name)
			}
			if got := normalizeNumpadText(msg).Text; got != "" {
				t.Fatalf("NumLock-off keypad key produced text %q", got)
			}

			info, ok := numpadNavigation(msg)
			if !ok || info.name != tt.name || info.nav != tt.nav || info.text != "" {
				t.Fatalf("numpadNavigation(Code=%v) = %+v, %v", tt.code, info, ok)
			}
			routeMsg := tea.KeyPressMsg{Code: tt.code, BaseCode: 'q', Mod: tea.ModCtrl, Text: "preserved"}
			fallback := info.navigationFallback(routeMsg)
			if fallback.Code != tt.nav || fallback.BaseCode != 0 {
				t.Fatalf("fallback = %#v, want Code=%v and no BaseCode", fallback, tt.nav)
			}
			if fallback.Mod != routeMsg.Mod || fallback.Text != routeMsg.Text {
				t.Fatalf("fallback lost event metadata: got %#v, started %#v", fallback, routeMsg)
			}
		})
	}
}

func TestUndocumentedNumpadKeysDoNotCollideWithOrdinaryBinds(t *testing.T) {
	for _, code := range []rune{tea.KeyKpEqual, tea.KeyKpComma, tea.KeyKpSep} {
		if got := keyToString(tea.KeyPressMsg{Code: code}); got != "" {
			t.Fatalf("keyToString(Code=%v) = %q, want no bind name", code, got)
		}
	}
}

// TestHomeEndEditInputWhileCtrlVariantsScroll pins the default key
// split: with no binds registered, bare Home/End fall through to the
// input widget as cursor movement, while Ctrl+Home/Ctrl+End hit the Go
// scroll fallback (the path that keeps degraded mode navigable).
func TestHomeEndEditInputWhileCtrlVariantsScroll(t *testing.T) {
	m := newTestModel(t)

	typed := "say hello"
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: typed})
	m = next.(*Model)

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("Home scrolled the viewport instead of reaching the input")
	}
	if pos := m.inputCtl.input.Position(); pos != 0 {
		t.Fatalf("Home left cursor at %d, want 0", pos)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("End scrolled the viewport instead of reaching the input")
	}
	if pos := m.inputCtl.input.Position(); pos != len(typed) {
		t.Fatalf("End left cursor at %d, want %d", pos, len(typed))
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
	m = next.(*Model)
	if m.output.viewport.Mode() == widget.ModeLive {
		t.Fatal("Ctrl+Home did not scroll the viewport to the top")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	m = next.(*Model)
	if m.output.viewport.Mode() != widget.ModeLive {
		t.Fatal("Ctrl+End did not return the viewport to live")
	}
	if got := m.inputCtl.input.Value(); got != typed {
		t.Fatalf("input draft = %q, want %q", got, typed)
	}
}
