package tui

import (
	"bufio"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

// Encodings Rune can receive for one key press. Bubble Tea requests xterm
// modifyOtherKeys 2 and kitty flag 1 (disambiguate); numpad mode adds kitty
// flags 8 and 16 (report all keys, associated text) and DECKPAM. Windows
// Terminal sets the Num Lock bit (+128) on every key under flag 8. win32 is
// the record form ultraviolet serializes from the Windows console API
// (terminal_reader_windows.go).
const (
	encLegacy    = "legacy"
	encMOK2      = "mok2"
	encKitty1    = "kitty1"
	encKitty25   = "kitty25"
	encKitty25NL = "kitty25+numlock"
	encWin32     = "win32"
)

// keyEncodingCases lists one key press per row with the bytes each encoding
// sends for it. Every encoding of a row must decode to the same name.
//
// Sources: "spec" is the kitty keyboard protocol document; "WT test" is
// Windows Terminal's kittyKeyboardProtocol.cpp table (French AZERTY), with
// the flag 16 text and Num Lock bits added as its encoder does; "WT" is
// derived from Windows Terminal 1.25.2733's encoder (terminalInput.cpp,
// _encodeKitty); "xterm" is xterm's documented CSI 27;mod;code~ form;
// "inferred" follows a documented format, but no source shows a terminal
// sending it. Legacy and kitty1 text rows are the typed UTF-8 text, which
// both protocols send for unmodified and Shift-only text keys. The win32
// row is ultraviolet's own serialization of a console input record.
var keyEncodingCases = []struct {
	name, want string
	enc        map[string]string
}{
	{"norwegian shift+| types §", "§", map[string]string{
		encLegacy: "§", encKitty1: "§",
		encKitty25: "\x1b[124;2;167u", encKitty25NL: "\x1b[124;130;167u", // WT
		encMOK2: "\x1b[27;2;167~", // inferred
	}},
	{"us shift+1 types !", "!", map[string]string{
		encLegacy: "!", encKitty1: "!",
		encKitty25: "\x1b[49;2;33u", encKitty25NL: "\x1b[49;130;33u", // WT
		encMOK2: "\x1b[27;2;33~", // inferred
	}},
	{"french shift+1 types !", "!", map[string]string{
		encLegacy: "!", encKitty25: "\x1b[224;2;33u", // WT test "K|T Shift+1"
	}},
	{"shift+a", "shift+a", map[string]string{
		encLegacy: "A", encKitty1: "A",
		encKitty25: "\x1b[97;2;65u", encKitty25NL: "\x1b[97;130;65u", // spec, WT test "K|T Shift+a"
		encMOK2: "\x1b[27;2;65~", // inferred
	}},
	{"caps lock a", "shift+a", map[string]string{
		encLegacy: "A", encKitty1: "A",
		encKitty25: "\x1b[97;65;65u", encKitty25NL: "\x1b[97;193;65u", // WT test "K CapsLock+NumLock+a"
	}},
	{"shift+ø", "shift+ø", map[string]string{
		encLegacy: "Ø", encKitty1: "Ø", encKitty25NL: "\x1b[248;130;216u", // WT
	}},
	{"cyrillic letter", "ф", map[string]string{
		encLegacy: "ф", encKitty1: "ф", encKitty25NL: "\x1b[1092;129;1092u", // WT
	}},
	{"altgr+2 types @", "@", map[string]string{
		encLegacy: "@", encKitty1: "@",
		encKitty25: "\x1b[64;;64u", encKitty25NL: "\x1b[64;129;64u", // WT: AltGr is a layer, not Ctrl+Alt
		encWin32: "\x1b[50;3;64;1;41;1_", // Left Ctrl, Right Alt, Num Lock
	}},
	{"dead key ¨ then e types ë", "ë", map[string]string{
		// WT: the dead key itself sends nothing. Whether its base key is
		// reported as "e" or "ë" is inferred; either names the press "ë".
		encLegacy: "ë", encKitty1: "ë", encKitty25NL: "\x1b[101;129;235u",
	}},
	{"space", "space", map[string]string{
		encLegacy: " ", encKitty1: " ", encKitty25NL: "\x1b[32;129;32u", // WT
	}},
	{"shift+space", "space", map[string]string{
		encLegacy: " ", encKitty1: " ", encKitty25NL: "\x1b[32;130;32u", // WT
	}},
	{"ctrl+a", "ctrl+a", map[string]string{
		encLegacy: "\x01", encKitty1: "\x1b[97;5u", // spec, WT test "D Ctrl+a"
		encKitty25NL: "\x1b[97;133u",  // WT test "K|T Ctrl+a"
		encMOK2:      "\x1b[27;5;97~", // xterm
	}},
	{"alt+a", "alt+a", map[string]string{
		encLegacy: "\x1ba", encKitty1: "\x1b[97;3u", // spec
		encKitty25NL: "\x1b[97;131;97u", // WT test "K Alt+a"
		encMOK2:      "\x1b[27;3;97~",   // xterm
	}},
	{"shift+tab", "shift+tab", map[string]string{
		encLegacy: "\x1b[Z", encKitty1: "\x1b[9;2u", encKitty25NL: "\x1b[9;130u", // WT test "K Shift+Tab"
	}},
	{"numpad 8", "numpad8", map[string]string{
		// Legacy is DECKPAM; kitty follows WT test "D Numpad5" with Num Lock.
		encLegacy: "\x1bOx", encKitty1: "\x1b[57407;129u", encKitty25NL: "\x1b[57407;129;56u",
	}},
}

// Known gaps:
//   - Alt with a shifted symbol is "alt+§" in legacy input (ESC §) but
//     "alt+shift+|" under kitty, which sends no text for Alt chords. Naming
//     it by the symbol needs the shifted key from ultraviolet.
//   - Kitty key number 0 ("CSI 0;;229u", text with no known key) decodes as
//     Ctrl+Space carrying the text and is named "ctrl+space".

func TestKeyNamesMatchAcrossEncodings(t *testing.T) {
	for _, tc := range keyEncodingCases {
		for _, enc := range slices.Sorted(maps.Keys(tc.enc)) {
			raw := tc.enc[enc]
			if names := decodedKeyNames(t, raw); !slices.Equal(names, []string{tc.want}) {
				t.Errorf("%s/%s: %q names %q, want %q", tc.name, enc, raw, names, tc.want)
			}
		}
	}
}

// decodedKeyNames runs raw terminal input through ultraviolet's decoder and
// returns the name of each key press. Modifier and lock key presses, which
// kitty flag 8 reports, are omitted.
func decodedKeyNames(t *testing.T, raw string) []string {
	t.Helper()
	var d uv.EventDecoder
	var names []string
	for buf := []byte(raw); len(buf) > 0; {
		n, ev := d.Decode(buf)
		if n == 0 {
			t.Fatalf("decoder stalled on %q", buf)
		}
		buf = buf[n:]
		key, ok := ev.(uv.KeyPressEvent)
		if !ok || (key.Code >= tea.KeyLeftShift && key.Code <= tea.KeyIsoLevel5Shift) ||
			key.Code == tea.KeyCapsLock || key.Code == tea.KeyNumLock || key.Code == tea.KeyScrollLock {
			continue
		}
		names = append(names, keyToString(tea.KeyPressMsg(key)))
	}
	return names
}

// TestCapturedKeyNamesMatch checks real captures written by keymatrix.py into
// testdata/keycaptures/*.jsonl, one object per line: {"mode", "layout",
// "label", "reads": [{"bytes": raw bytes as latin-1}]}. A label must name the
// same keys with numpad mode off and on. Numpad labels ("np...") are exempt:
// numpad mode exists to tell those keys apart.
func TestCapturedKeyNamesMatch(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "keycaptures", "*.jsonl"))
	if len(files) == 0 {
		t.Skip("no captures in testdata/keycaptures")
	}
	names := map[string]map[string][]string{} // layout/label -> mode -> names
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lines := bufio.NewScanner(strings.NewReader(string(data)))
		for lines.Scan() {
			var rec struct {
				Mode, Layout, Label string
				Reads               []struct{ Bytes string }
			}
			if err := json.Unmarshal(lines.Bytes(), &rec); err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			if (rec.Mode != "off" && rec.Mode != "on") || strings.HasPrefix(rec.Label, "_") ||
				strings.HasPrefix(rec.Label, "np") {
				continue
			}
			var raw []byte
			for _, read := range rec.Reads {
				for _, r := range read.Bytes {
					raw = append(raw, byte(r))
				}
			}
			key := rec.Layout + "/" + rec.Label
			if names[key] == nil {
				names[key] = map[string][]string{}
			}
			names[key][rec.Mode] = decodedKeyNames(t, string(raw))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(names)) {
		off, on := names[key]["off"], names[key]["on"]
		t.Logf("%s: off %q, on %q", key, off, on)
		if off != nil && on != nil && !slices.Equal(off, on) {
			t.Errorf("%s: numpad mode off names %q, on names %q", key, off, on)
		}
	}
}

// A Norwegian bind on "§" fires whether the press arrives as text (numpad
// mode off) or as kitty flag 25's layout key, Shift, and text (numpad mode
// on). The decoded name was "shift+|" in the second case.
func TestTextBindFiresInBothNumpadModes(t *testing.T) {
	for _, raw := range []string{"§", "\x1b[124;130;167u"} {
		h := newControllerHarness()
		h.ctl.input.SetBindings(input.Bindings{"§": {Enabled: true}})
		h.ctl.HandleKey(decodeKeyPress(t, raw))
		if binds := h.executeBinds(); !slices.Equal(binds, []ui.ExecuteBindMsg{"§"}) || h.ctl.input.Value() != "" {
			t.Fatalf("%q: binds %v, draft %q", raw, binds, h.ctl.input.Value())
		}
	}
}

func TestTextInputActionFiresInBothNumpadModes(t *testing.T) {
	for _, raw := range []string{"!", "\x1b[49;130;33u"} {
		h := newControllerHarness()
		h.ctl.input.SetBindings(input.Bindings{"!": {Action: "input.toggle_mode", Enabled: true}})
		before := h.ctl.input.SubmissionMode()
		h.ctl.HandleKey(decodeKeyPress(t, raw))
		if h.ctl.input.SubmissionMode() == before || h.ctl.input.Value() != "" {
			t.Fatalf("%q: mode stayed %v, draft %q", raw, before, h.ctl.input.Value())
		}
	}
}

// One name per press: the layout key and Shift do not also name a text key,
// or the bind would again depend on numpad mode.
func TestTextKeyHasNoPhysicalActionName(t *testing.T) {
	h := newControllerHarness()
	h.ctl.input.SetBindings(input.Bindings{"shift+1": {Action: "input.toggle_mode", Enabled: true}})
	before := h.ctl.input.SubmissionMode()
	h.ctl.HandleKey(decodeKeyPress(t, "\x1b[49;130;33u"))
	if h.ctl.input.SubmissionMode() != before || h.ctl.input.Value() != "!" {
		t.Fatalf("shift+1 action ran: mode %v, draft %q", h.ctl.input.SubmissionMode(), h.ctl.input.Value())
	}
}

func decodeKeyPress(t *testing.T, raw string) tea.KeyPressMsg {
	t.Helper()
	var d uv.EventDecoder
	n, ev := d.Decode([]byte(raw))
	key, ok := ev.(uv.KeyPressEvent)
	if !ok || n != len(raw) {
		t.Fatalf("%q decodes to %#v after %d bytes", raw, ev, n)
	}
	return tea.KeyPressMsg(key)
}
