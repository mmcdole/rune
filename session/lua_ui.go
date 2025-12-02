package session

import "github.com/drake/rune/ui"

// Print implements lua.UIService.
func (s *Session) Print(text string) {
	s.ui.Print(text)
}

// PaneCreate implements lua.UIService.
func (s *Session) PaneCreate(name string) {
	s.ui.CreatePane(name)
}

// PaneWrite implements lua.UIService.
func (s *Session) PaneWrite(name, text string) {
	s.ui.WritePane(name, text)
}

// PaneToggle implements lua.UIService.
func (s *Session) PaneToggle(name string) {
	s.ui.TogglePane(name)
}

// PaneClear implements lua.UIService.
func (s *Session) PaneClear(name string) {
	s.ui.ClearPane(name)
}

// ShowPicker implements lua.UIService.
func (s *Session) ShowPicker(title string, items []ui.PickerItem, onSelect func(string), inline bool) {
	id := s.callbacks.Register(onSelect)
	s.ui.ShowPicker(title, items, id, inline)
}

// GetInput implements lua.UIService.
func (s *Session) GetInput() string {
	return s.currentInput
}

// SetInput implements lua.UIService.
func (s *Session) SetInput(text string) {
	s.ui.SetInput(text)
	s.currentInput = text
}
