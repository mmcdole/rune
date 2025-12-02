package input

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Model handles text input.
// This is a dumb text box - modes, history navigation, and completion logic
// belong in the parent Model.
type Model struct {
	textinput textinput.Model
	width     int
}

// New creates a new input model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0 // No limit
	ti.Width = 80
	ti.Focus()

	// Enable native suggestions (ghost text)
	ti.ShowSuggestions = true

	return Model{
		textinput: ti,
	}
}

// SetWidth updates the input width.
func (m *Model) SetWidth(w int) {
	m.width = w
	m.textinput.Width = w - 2 // Account for prompt
}

// Focus gives focus to the input.
func (m *Model) Focus() {
	m.textinput.Focus()
}

// Blur removes focus from the input.
func (m *Model) Blur() {
	m.textinput.Blur()
}

// Value returns the current input text.
func (m *Model) Value() string {
	return m.textinput.Value()
}

// SetValue sets the input text.
func (m *Model) SetValue(s string) {
	m.textinput.SetValue(s)
}

// CursorEnd moves the cursor to the end of the input.
func (m *Model) CursorEnd() {
	m.textinput.CursorEnd()
}

// Position returns the current cursor position.
func (m *Model) Position() int {
	return m.textinput.Position()
}

// SetCursor sets the cursor position.
func (m *Model) SetCursor(pos int) {
	m.textinput.SetCursor(pos)
}

// SetSuggestions sets the autocomplete suggestions.
func (m *Model) SetSuggestions(suggestions []string) {
	m.textinput.SetSuggestions(suggestions)
}

// ClearSuggestions clears the autocomplete suggestions.
func (m *Model) ClearSuggestions() {
	m.textinput.SetSuggestions(nil)
}

// Reset clears the input.
func (m *Model) Reset() {
	m.textinput.SetValue("")
	m.textinput.SetSuggestions(nil)
}

// Update handles tea messages for the input.
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	var cmd tea.Cmd
	m.textinput, cmd = m.textinput.Update(msg)
	return m, cmd
}

// View renders the input line.
func (m *Model) View() string {
	return m.textinput.View()
}
