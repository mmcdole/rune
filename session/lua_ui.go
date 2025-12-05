package session

import (
	"fmt"

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
func (s *Session) ShowPicker(title string, items []ui.PickerItem, onSelect func(string), inline bool) {
	id := s.registerPickerCallback(onSelect)
	s.ui.ShowPicker(title, items, id, inline)
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

// --- Picker callback management ---

// registerPickerCallback stores a callback and returns its ID.
func (s *Session) registerPickerCallback(fn func(string)) string {
	s.pickerNextID++
	id := fmt.Sprintf("p%d", s.pickerNextID)
	s.pickerCallbacks[id] = fn
	return id
}

// executePickerCallback runs and removes a callback by ID.
func (s *Session) executePickerCallback(id string, value string, accepted bool) {
	cb, ok := s.pickerCallbacks[id]
	if !ok {
		return
	}
	delete(s.pickerCallbacks, id)
	if accepted && cb != nil {
		cb(value)
	}
}
