package tui

import (
	"bytes"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
)

func TestKeypadModeSequence(t *testing.T) {
	if got := keypadModeSequence(true); got != ansi.KeypadApplicationMode {
		t.Fatalf("enabled keypad sequence = %q, want %q", got, ansi.KeypadApplicationMode)
	}
	if got := keypadModeSequence(false); got != ansi.KeypadNumericMode {
		t.Fatalf("disabled keypad sequence = %q, want %q", got, ansi.KeypadNumericMode)
	}
}

func TestUpdateConfigQueuesKeypadModeOnlyWhenItChanges(t *testing.T) {
	b := NewBubbleTeaUI()

	b.UpdateConfig(ui.Config{Numpad: false})
	if msg := <-b.msgQueue; msg != (ui.UpdateConfigMsg{Numpad: false}) {
		t.Fatalf("first queued message = %#v, want disabled config", msg)
	}
	if msg := <-b.msgQueue; msg != (tea.RawMsg{Msg: ansi.KeypadNumericMode}) {
		t.Fatalf("second queued message = %#v, want numeric keypad mode", msg)
	}

	b.UpdateConfig(ui.Config{KeepInput: true, Numpad: false})
	if msg := <-b.msgQueue; msg != (ui.UpdateConfigMsg{KeepInput: true, Numpad: false}) {
		t.Fatalf("unchanged keypad config message = %#v", msg)
	}
	if got := len(b.msgQueue); got != 0 {
		t.Fatalf("unchanged keypad mode queued %d extra messages", got)
	}

	b.UpdateConfig(ui.Config{KeepInput: true, Numpad: true})
	if msg := <-b.msgQueue; msg != (ui.UpdateConfigMsg{KeepInput: true, Numpad: true}) {
		t.Fatalf("enabled keypad config message = %#v", msg)
	}
	if msg := <-b.msgQueue; msg != (tea.RawMsg{Msg: ansi.KeypadApplicationMode}) {
		t.Fatalf("enabled keypad raw message = %#v, want application mode", msg)
	}
}

func TestWriteKeypadModeUsesProgramOutput(t *testing.T) {
	b := NewBubbleTeaUI()
	var output bytes.Buffer
	b.output = &output

	if err := b.writeKeypadMode(true); err != nil {
		t.Fatal(err)
	}
	if err := b.writeKeypadMode(false); err != nil {
		t.Fatal(err)
	}

	want := ansi.KeypadApplicationMode + ansi.KeypadNumericMode
	if got := output.String(); got != want {
		t.Fatalf("keypad lifecycle output = %q, want %q", got, want)
	}
}

func TestNormalizeEditorTextPreservesWhitespace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "indentation and trailing spaces", in: "  first  \n\tsecond \n", want: "  first  \n\tsecond "},
		{name: "one blank line remains", in: "first\n\n", want: "first\n"},
		{name: "CRLF", in: "first\r\n\tsecond\r\n", want: "first\n\tsecond"},
		{name: "bare CR", in: "first\rsecond\r", want: "first\nsecond"},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeEditorText(tt.in); got != tt.want {
				t.Fatalf("normalizeEditorText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
