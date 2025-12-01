package input

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// CommandPrompt handles text input.
// This is a dumb text box - modes, history navigation, and completion logic
// belong in the parent Model.
type CommandPrompt struct {
	textinput textinput.Model
	width     int
}

// New creates a new command prompt.
func New() CommandPrompt {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0 // No limit
	ti.Width = 80
	ti.Focus()

	// Enable native suggestions (ghost text)
	ti.ShowSuggestions = true

	return CommandPrompt{
		textinput: ti,
	}
}

// SetWidth updates the input width.
func (m *CommandPrompt) SetWidth(w int) {
	m.width = w
	m.textinput.Width = w - 2 // Account for prompt
}

// Focus gives focus to the input.
func (m *CommandPrompt) Focus() {
	m.textinput.Focus()
}

// Blur removes focus from the input.
func (m *CommandPrompt) Blur() {
	m.textinput.Blur()
}

// Value returns the current input text.
func (m *CommandPrompt) Value() string {
	return m.textinput.Value()
}

// SetValue sets the input text.
func (m *CommandPrompt) SetValue(s string) {
	m.textinput.SetValue(s)
}

// CursorEnd moves the cursor to the end of the input.
func (m *CommandPrompt) CursorEnd() {
	m.textinput.CursorEnd()
}

// Position returns the current cursor position.
func (m *CommandPrompt) Position() int {
	return m.textinput.Position()
}

// SetCursor sets the cursor position.
func (m *CommandPrompt) SetCursor(pos int) {
	m.textinput.SetCursor(pos)
}

// SetSuggestions sets the autocomplete suggestions.
func (m *CommandPrompt) SetSuggestions(suggestions []string) {
	m.textinput.SetSuggestions(suggestions)
}

// ClearSuggestions clears the autocomplete suggestions.
func (m *CommandPrompt) ClearSuggestions() {
	m.textinput.SetSuggestions(nil)
}

// Reset clears the input.
func (m *CommandPrompt) Reset() {
	m.textinput.SetValue("")
	m.textinput.SetSuggestions(nil)
}

// Update handles tea messages for the input.
func (m *CommandPrompt) Update(msg tea.Msg) (*CommandPrompt, tea.Cmd) {
	var cmd tea.Cmd
	m.textinput, cmd = m.textinput.Update(msg)
	return m, cmd
}

// View renders the input line.
func (m *CommandPrompt) View() string {
	return m.textinput.View()
}
