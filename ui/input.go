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

	// Tab completion
	completer *WordCache // Word cache for completions
}

// NewInputModel creates a new input model with the given word cache for completion.
func NewInputModel(completer *WordCache) InputModel {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0 // No limit
	ti.Width = 80
	ti.Focus()

	// Enable native suggestions (ghost text)
	// Tab = accept, Ctrl+N/P = cycle
	ti.ShowSuggestions = true

	return InputModel{
		textinput:    ti,
		history:      make([]string, 0, 1000),
		historyIndex: -1,
		historyLimit: 10000,
		mode:         ModeNormal,
		completer:    completer,
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

// CursorEnd moves the cursor to the end of the input.
func (m *InputModel) CursorEnd() {
	m.textinput.CursorEnd()
}

// ClearSuggestions clears the autocomplete suggestions.
func (m *InputModel) ClearSuggestions() {
	m.textinput.SetSuggestions(nil)
}

// Reset clears the input and resets history navigation.
func (m *InputModel) Reset() {
	m.textinput.SetValue("")
	m.textinput.SetSuggestions(nil) // Clear ghost text
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
// If there's a prefix typed, searches for history matching that prefix.
func (m *InputModel) HistoryUp() {
	if len(m.history) == 0 {
		return
	}

	if m.historyIndex == -1 {
		// Save current input as draft before entering history
		m.draft = m.textinput.Value()
	}

	// If we have a prefix (draft), search for matching history
	if m.draft != "" {
		start := m.historyIndex - 1
		if m.historyIndex == -1 {
			start = len(m.history) - 1
		}
		for i := start; i >= 0; i-- {
			if strings.HasPrefix(m.history[i], m.draft) {
				m.historyIndex = i
				m.textinput.SetValue(m.history[i])
				m.textinput.CursorEnd()
				return
			}
		}
		// No match found, stay where we are
		return
	}

	// No prefix - cycle through all history
	if m.historyIndex == -1 {
		m.historyIndex = len(m.history) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}

	m.textinput.SetValue(m.history[m.historyIndex])
	m.textinput.CursorEnd()
}

// HistoryDown moves down through history, restoring draft at bottom.
// If there's a prefix typed, searches for history matching that prefix.
func (m *InputModel) HistoryDown() {
	if m.historyIndex == -1 {
		return // Already at draft
	}

	// If we have a prefix (draft), search for matching history
	if m.draft != "" {
		for i := m.historyIndex + 1; i < len(m.history); i++ {
			if strings.HasPrefix(m.history[i], m.draft) {
				m.historyIndex = i
				m.textinput.SetValue(m.history[i])
				m.textinput.CursorEnd()
				return
			}
		}
		// No more matches - return to draft
		m.historyIndex = -1
		m.textinput.SetValue(m.draft)
		m.textinput.CursorEnd()
		return
	}

	// No prefix - cycle through all history
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
			m.updateSuggestions()
			return m, nil
		case tea.KeyDown:
			m.HistoryDown()
			m.updateSuggestions()
			return m, nil
		case tea.KeyCtrlU:
			m.textinput.SetValue("")
			return m, nil
		case tea.KeyCtrlW:
			val := m.textinput.Value()
			pos := m.textinput.Position()
			if pos > 0 {
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

	// Let textinput handle the key (including Tab for suggestions)
	m.textinput, cmd = m.textinput.Update(msg)

	// Update suggestions based on current word
	m.updateSuggestions()

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

// updateSuggestions updates the textinput suggestions based on current word.
func (m *InputModel) updateSuggestions() {
	if m.completer == nil {
		return
	}

	val := m.textinput.Value()
	if val == "" {
		m.textinput.SetSuggestions(nil)
		return
	}

	// Find the word at/before cursor
	pos := m.textinput.Position()
	start, end := m.findWordBoundaries(val, pos)
	if start == end {
		m.textinput.SetSuggestions(nil)
		return
	}

	prefix := val[start:end]
	if len(prefix) < 2 {
		m.textinput.SetSuggestions(nil)
		return
	}

	matches := m.completer.FindMatches(prefix)
	if len(matches) == 0 {
		m.textinput.SetSuggestions(nil)
		return
	}

	// Build full-line suggestions by replacing the current word
	before := val[:start]
	after := ""
	if end < len(val) {
		after = val[end:]
	}

	suggestions := make([]string, 0, len(matches))
	for _, match := range matches {
		suggestions = append(suggestions, before+match+after)
	}

	m.textinput.SetSuggestions(suggestions)
}

// findWordBoundaries finds the start and end of the word at or before cursor.
func (m *InputModel) findWordBoundaries(text string, cursor int) (int, int) {
	if cursor > len(text) {
		cursor = len(text)
	}

	if cursor == 0 {
		return 0, 0
	}

	// Check if we're right after a space (no word at cursor)
	if text[cursor-1] == ' ' {
		return cursor, cursor
	}

	// Scan back for word start
	start := cursor
	for start > 0 && text[start-1] != ' ' {
		start--
	}

	// Scan forward for word end (from cursor)
	end := cursor
	for end < len(text) && text[end] != ' ' {
		end++
	}

	return start, end
}
