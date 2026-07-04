package session

import (
	"unicode/utf8"

	"github.com/mmcdole/rune/ui"
)

// Print implements lua.Host.
func (s *Session) Print(text string) {
	s.ui.Print(text)
}

// PaneCreate implements lua.Host.
func (s *Session) PaneCreate(name string) {
	s.ui.CreatePane(name)
}

// PaneWrite implements lua.Host.
func (s *Session) PaneWrite(name, text string) {
	s.ui.WritePane(name, text)
}

// PaneToggle implements lua.Host.
func (s *Session) PaneToggle(name string) {
	s.ui.TogglePane(name)
}

// PaneClear implements lua.Host.
func (s *Session) PaneClear(name string) {
	s.ui.ClearPane(name)
}

// ShowPicker implements lua.Host.
func (s *Session) ShowPicker(opts ui.ShowPickerMsg) {
	s.ui.ShowPicker(opts)
}

// GetInput implements lua.Host.
func (s *Session) GetInput() string {
	return s.currentInput
}

// SetInput implements lua.Host.
func (s *Session) SetInput(text string) {
	s.ui.SetInput(text)
	s.currentInput = text
}

// InputGetCursor implements lua.Host.
func (s *Session) InputGetCursor() int {
	return s.currentCursor
}

// InputSetCursor implements lua.Host.
// The position is clamped to the current input's rune count here, so
// the mirror Lua reads via rune.input.get_cursor() cannot drift from
// what the input widget (which clamps independently) will display.
func (s *Session) InputSetCursor(pos int) {
	if pos < 0 {
		pos = 0
	}
	if max := utf8.RuneCountInString(s.currentInput); pos > max {
		pos = max
	}
	s.currentCursor = pos
	s.ui.InputSetCursor(pos)
}

// OpenEditor implements lua.Host.
func (s *Session) OpenEditor(initial string) (string, bool) {
	return s.ui.OpenEditor(initial)
}

// PaneScrollUp implements lua.Host.
func (s *Session) PaneScrollUp(name string, lines int) {
	s.ui.PaneScrollUp(name, lines)
}

// PaneScrollDown implements lua.Host.
func (s *Session) PaneScrollDown(name string, lines int) {
	s.ui.PaneScrollDown(name, lines)
}

// PaneScrollToTop implements lua.Host.
func (s *Session) PaneScrollToTop(name string) {
	s.ui.PaneScrollToTop(name)
}

// PaneScrollToBottom implements lua.Host.
func (s *Session) PaneScrollToBottom(name string) {
	s.ui.PaneScrollToBottom(name)
}

// handlePickerResult delegates to Engine for callback execution or cancellation.
func (s *Session) handlePickerResult(id string, value string, accepted bool) {
	if accepted {
		s.engine.ExecutePickerCallback(id, value)
	} else {
		s.engine.CancelPickerCallback(id)
	}
}
