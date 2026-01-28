package session

import (
	"github.com/drake/rune/ui"
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
func (s *Session) ShowPicker(title string, items []ui.PickerItem, callbackID string, inline bool) {
	s.ui.ShowPicker(title, items, callbackID, inline)
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
func (s *Session) InputSetCursor(pos int) {
	s.currentCursor = pos
	s.ui.InputSetCursor(pos)
}

// SetGhost implements lua.Host.
// Sets ghost text for command-level suggestions.
func (s *Session) SetGhost(text string) {
	s.ui.SetGhost(text)
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
