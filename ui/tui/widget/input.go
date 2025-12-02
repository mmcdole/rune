package widget

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
)

// Input handles the input area including text entry, picker overlay, and borders.
type Input struct {
	textinput textinput.Model
	picker    *Picker[ui.PickerItem]
	styles    style.Styles

	// State
	pickerActive bool
	pickerCB     string // Callback ID for picker selection
	width        int

	// Tab completion
	wordCache *util.CompletionEngine
}

// NewInput creates a new input widget.
func NewInput(styles style.Styles) *Input {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0
	ti.Width = 80
	ti.Focus()
	ti.ShowSuggestions = true

	return &Input{
		textinput: ti,
		picker: NewPicker[ui.PickerItem](PickerConfig{
			MaxVisible: 10,
			EmptyText:  "No matches",
		}, styles),
		styles:    styles,
		wordCache: util.NewCompletionEngine(5000),
	}
}

// Init implements tea.Model.
func (i *Input) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (i *Input) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	i.textinput, cmd = i.textinput.Update(msg)
	return i, cmd
}

// View implements tea.Model.
func (i *Input) View() string {
	var parts []string

	// Picker overlay (if active)
	if i.pickerActive {
		parts = append(parts, i.picker.View())
	}

	// Top border
	parts = append(parts, i.borderLine())

	// Input field
	parts = append(parts, i.textinput.View())

	// Bottom border
	parts = append(parts, i.borderLine())

	return strings.Join(parts, "\n")
}

// SetWidth implements Widget.
func (i *Input) SetWidth(w int) {
	i.width = w
	i.textinput.Width = w - 2 // Account for prompt
	i.picker.SetWidth(w)
}

// Height implements Widget.
func (i *Input) Height() int {
	h := 3 // top border + input + bottom border
	if i.pickerActive {
		h += i.picker.Height()
	}
	return h
}

func (i *Input) borderLine() string {
	return "\x1b[90m" + strings.Repeat("─", i.width) + "\x1b[0m"
}

// Value returns the current input text.
func (i *Input) Value() string {
	return i.textinput.Value()
}

// SetValue sets the input text.
func (i *Input) SetValue(s string) {
	i.textinput.SetValue(s)
}

// CursorEnd moves the cursor to the end.
func (i *Input) CursorEnd() {
	i.textinput.CursorEnd()
}

// Position returns the cursor position.
func (i *Input) Position() int {
	return i.textinput.Position()
}

// SetCursor sets the cursor position.
func (i *Input) SetCursor(pos int) {
	i.textinput.SetCursor(pos)
}

// Reset clears the input.
func (i *Input) Reset() {
	i.textinput.SetValue("")
	i.textinput.SetSuggestions(nil)
}

// Focus gives focus to the input.
func (i *Input) Focus() {
	i.textinput.Focus()
}

// Blur removes focus from the input.
func (i *Input) Blur() {
	i.textinput.Blur()
}

// SetSuggestions sets tab completion suggestions.
func (i *Input) SetSuggestions(suggestions []string) {
	i.textinput.SetSuggestions(suggestions)
}

// UpdateSuggestions refreshes suggestions based on current input.
func (i *Input) UpdateSuggestions() {
	val := i.textinput.Value()
	if val == "" {
		i.textinput.SetSuggestions(nil)
		return
	}

	pos := i.textinput.Position()
	start, end := util.FindWordBoundaries(val, pos)
	if start == end {
		i.textinput.SetSuggestions(nil)
		return
	}

	prefix := val[start:end]
	if len(prefix) < 2 {
		i.textinput.SetSuggestions(nil)
		return
	}

	matches := i.wordCache.FindMatches(prefix)
	if len(matches) == 0 {
		i.textinput.SetSuggestions(nil)
		return
	}

	// Build full-line suggestions
	before := val[:start]
	after := ""
	if end < len(val) {
		after = val[end:]
	}

	suggestions := make([]string, 0, len(matches))
	for _, match := range matches {
		suggestions = append(suggestions, before+match+after)
	}

	i.textinput.SetSuggestions(suggestions)
}

// AddToWordCache adds words from a line to the completion cache.
func (i *Input) AddToWordCache(line string) {
	i.wordCache.AddLine(line)
}

// AddInputToWordCache adds user input to the completion cache.
func (i *Input) AddInputToWordCache(input string) {
	i.wordCache.AddInput(input)
}

// DeleteWord deletes the word before cursor.
func (i *Input) DeleteWord() {
	val := i.textinput.Value()
	pos := i.textinput.Position()
	if pos > 0 {
		newPos := pos - 1
		for newPos > 0 && val[newPos-1] == ' ' {
			newPos--
		}
		for newPos > 0 && val[newPos-1] != ' ' {
			newPos--
		}
		i.textinput.SetValue(val[:newPos] + val[pos:])
		i.textinput.SetCursor(newPos)
	}
}

// Picker access

// PickerActive returns true if picker is visible.
func (i *Input) PickerActive() bool {
	return i.pickerActive
}

// PickerCallbackID returns the current picker callback ID.
func (i *Input) PickerCallbackID() string {
	return i.pickerCB
}

// ShowPicker displays the picker with items.
func (i *Input) ShowPicker(items []ui.PickerItem, title string, callbackID string, inline bool) {
	i.picker.SetItems(items)
	i.pickerCB = callbackID
	i.pickerActive = true

	if inline {
		i.picker.SetHeader("")
		i.picker.Filter(i.textinput.Value())
	} else {
		header := title
		if header != "" {
			header += ": "
		}
		i.picker.SetHeader(header)
		i.picker.Filter("")
	}
}

// HidePicker closes the picker.
func (i *Input) HidePicker() {
	i.pickerActive = false
	i.picker.Reset()
}

// PickerSelectUp moves picker selection up.
func (i *Input) PickerSelectUp() {
	i.picker.SelectUp()
}

// PickerSelectDown moves picker selection down.
func (i *Input) PickerSelectDown() {
	i.picker.SelectDown()
}

// PickerSelected returns the selected picker item.
func (i *Input) PickerSelected() (ui.PickerItem, bool) {
	return i.picker.Selected()
}

// PickerFilter updates the picker filter.
func (i *Input) PickerFilter(query string) {
	i.picker.Filter(query)
}

// PickerQuery returns the picker's current query.
func (i *Input) PickerQuery() string {
	return i.picker.Query()
}

// UpdatePickerFilter updates filter based on input value.
func (i *Input) UpdatePickerFilter() {
	val := i.textinput.Value()
	if val == "" {
		i.HidePicker()
		return
	}
	i.picker.Filter(val)
}
