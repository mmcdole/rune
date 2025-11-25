package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// InputMode represents the current input state.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeSlash  // Slash command picker active
	ModeFuzzy  // Ctrl+R fuzzy search active
)

// InputModel handles text input with history and mode management.
type InputModel struct {
	textinput    textinput.Model
	history      []string
	historyIndex int    // -1 = draft, 0..n = history position
	historyLimit int    // Max history entries
	draft        string // Preserved when browsing history
	mode         InputMode
	width        int
}

// NewInputModel creates a new input model.
func NewInputModel() InputModel {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0 // No limit
	ti.Width = 80
	ti.Focus()

	return InputModel{
		textinput:    ti,
		history:      make([]string, 0, 1000),
		historyIndex: -1,
		historyLimit: 10000,
		mode:         ModeNormal,
	}
}

// SetWidth updates the input width.
func (m *InputModel) SetWidth(w int) {
	m.width = w
	m.textinput.Width = w - 2 // Account for prompt
}

// Focus gives focus to the input.
func (m *InputModel) Focus() {
	m.textinput.Focus()
}

// Blur removes focus from the input.
func (m *InputModel) Blur() {
	m.textinput.Blur()
}

// Value returns the current input text.
func (m *InputModel) Value() string {
	return m.textinput.Value()
}

// SetValue sets the input text.
func (m *InputModel) SetValue(s string) {
	m.textinput.SetValue(s)
}

// Reset clears the input and resets history navigation.
func (m *InputModel) Reset() {
	m.textinput.SetValue("")
	m.historyIndex = -1
	m.draft = ""
}

// AddToHistory adds a command to history.
func (m *InputModel) AddToHistory(cmd string) {
	if cmd == "" {
		return
	}
	// Don't add duplicates of the last command
	if len(m.history) > 0 && m.history[len(m.history)-1] == cmd {
		return
	}
	m.history = append(m.history, cmd)
	// Trim if over limit
	if len(m.history) > m.historyLimit {
		m.history = m.history[len(m.history)-m.historyLimit:]
	}
}

// HistoryUp moves up through history, preserving draft on first move.
func (m *InputModel) HistoryUp() {
	if len(m.history) == 0 {
		return
	}

	if m.historyIndex == -1 {
		// Save current input as draft before entering history
		m.draft = m.textinput.Value()
		m.historyIndex = len(m.history) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}

	m.textinput.SetValue(m.history[m.historyIndex])
	m.textinput.CursorEnd()
}

// HistoryDown moves down through history, restoring draft at bottom.
func (m *InputModel) HistoryDown() {
	if m.historyIndex == -1 {
		return // Already at draft
	}

	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.textinput.SetValue(m.history[m.historyIndex])
	} else {
		// Return to draft
		m.historyIndex = -1
		m.textinput.SetValue(m.draft)
	}
	m.textinput.CursorEnd()
}

// Mode returns the current input mode.
func (m *InputModel) Mode() InputMode {
	return m.mode
}

// SetMode changes the input mode.
func (m *InputModel) SetMode(mode InputMode) {
	m.mode = mode
}

// History returns a copy of the command history.
func (m *InputModel) History() []string {
	result := make([]string, len(m.history))
	copy(result, m.history)
	return result
}

// Update handles key events for the input.
func (m *InputModel) Update(msg tea.Msg) (*InputModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.HistoryUp()
			return m, nil
		case tea.KeyDown:
			m.HistoryDown()
			return m, nil
		case tea.KeyCtrlU:
			// Clear line
			m.textinput.SetValue("")
			return m, nil
		case tea.KeyCtrlW:
			// Delete word
			val := m.textinput.Value()
			pos := m.textinput.Position()
			if pos > 0 {
				// Find word boundary
				newPos := pos - 1
				for newPos > 0 && val[newPos-1] == ' ' {
					newPos--
				}
				for newPos > 0 && val[newPos-1] != ' ' {
					newPos--
				}
				m.textinput.SetValue(val[:newPos] + val[pos:])
				m.textinput.SetCursor(newPos)
			}
			return m, nil
		}
	}

	m.textinput, cmd = m.textinput.Update(msg)
	return m, cmd
}

// View renders the input line.
func (m *InputModel) View() string {
	return m.textinput.View()
}

// StartsWithSlash returns true if the input starts with "/".
func (m *InputModel) StartsWithSlash() bool {
	return strings.HasPrefix(m.textinput.Value(), "/")
}

// GetSlashQuery returns the text after "/" for filtering slash commands.
func (m *InputModel) GetSlashQuery() string {
	val := m.textinput.Value()
	if strings.HasPrefix(val, "/") {
		return val[1:]
	}
	return ""
}
