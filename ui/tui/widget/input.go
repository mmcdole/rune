package widget

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/style"
)

// Compile-time check that Input implements Widget
var _ Widget = (*Input)(nil)

// Input handles the input area including text entry, picker overlay, and borders.
type Input struct {
	textinput textinput.Model
	picker    *Picker
	styles    style.Styles

	// State
	pickerActive bool
	pickerCB     string // Callback ID for picker selection
	width        int

	// Ghost text (command-level suggestion from Lua)
	ghostText string
}

// NewInput creates a new input widget.
func NewInput(styles style.Styles) *Input {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0
	ti.Width = 80
	ti.Focus()

	return &Input{
		textinput: ti,
		picker: NewPicker(PickerConfig{
			MaxVisible: 10,
			EmptyText:  "No matches",
		}, styles),
		styles: styles,
	}
}

// UpdateTextInput forwards messages to the underlying textinput.
func (i *Input) UpdateTextInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	i.textinput, cmd = i.textinput.Update(msg)
	return cmd
}

// View implements Widget.
func (i *Input) View() string {
	var parts []string

	// Picker overlay (if active)
	if i.pickerActive {
		parts = append(parts, i.picker.View())
	}

	// Top border
	parts = append(parts, i.borderLine())

	// Input field with ghost text
	currentVal := i.textinput.Value()
	cursorPos := i.textinput.Position()

	// Check if we should render ghost text
	hasGhost := i.ghostText != "" &&
		strings.HasPrefix(i.ghostText, currentVal) &&
		len(i.ghostText) > len(currentVal) &&
		cursorPos == len(currentVal) // Only show ghost when cursor is at end

	var inputView string
	if hasGhost {
		// Build custom view with ghost text integrated at cursor position
		// This makes the cursor appear ON the first ghost char (like fish shell)
		remainder := i.ghostText[len(currentVal):]
		prompt := i.textinput.Prompt

		// Render: prompt + typed text + cursor on first ghost char + rest of ghost dim
		if len(remainder) > 0 {
			firstGhostRune := []rune(remainder)[0]
			restGhost := string([]rune(remainder)[1:])
			// \x1b[7m = inverse (cursor), \x1b[90m = dim gray
			inputView = prompt + currentVal +
				"\x1b[7;90m" + string(firstGhostRune) + "\x1b[27m" + // cursor on first ghost char
				"\x1b[90m" + restGhost + "\x1b[0m" // rest of ghost dim
		}
	} else {
		// Use normal textinput view (with its own cursor)
		inputView = i.textinput.View()
	}

	parts = append(parts, inputView)

	// Bottom border
	parts = append(parts, i.borderLine())

	return strings.Join(parts, "\n")
}

// SetSize implements Widget.
func (i *Input) SetSize(width, height int) {
	i.width = width
	i.textinput.Width = width - 2 // Account for prompt
	i.picker.SetWidth(width)
	// height is ignored - input has intrinsic height
}

// PreferredHeight implements Widget.
func (i *Input) PreferredHeight() int {
	h := 3 // top border + input + bottom border
	if i.pickerActive {
		h += i.picker.PreferredHeight()
	}
	return h
}

func (i *Input) borderLine() string {
	return style.RenderBorder(i.width)
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
	i.ghostText = ""
}

// Focus gives focus to the input.
func (i *Input) Focus() {
	i.textinput.Focus()
}

// Blur removes focus from the input.
func (i *Input) Blur() {
	i.textinput.Blur()
}

// SetGhostText sets the ghost suggestion text (visual only).
// Go just renders; Lua is the source of truth for what to suggest.
func (i *Input) SetGhostText(text string) {
	i.ghostText = text
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
