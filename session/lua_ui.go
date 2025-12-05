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

// executePickerCallback delegates to Engine for callback execution.
func (s *Session) executePickerCallback(id string, value string, accepted bool) {
	if accepted {
		s.engine.ExecutePickerCallback(id, value)
	} else {
		s.engine.CancelPickerCallback(id)
	}
}
