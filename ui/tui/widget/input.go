package widget

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
)

// Compile-time check that Input implements Widget
var _ Widget = (*Input)(nil)

// Input handles the input area including text entry, picker overlay, and borders.
type Input struct {
	textinput textinput.Model
	picker    *Picker
	styles    style.Styles

	// State
	pickerActive  bool
	pickerCB      string // Callback ID for picker selection
	pickerDismiss bool   // Close inline picker once input contains a space
	width         int
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

	// Input field
	parts = append(parts, i.textinput.View())

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
}

// Picker access

// PickerCallbackID returns the current picker callback ID.
func (i *Input) PickerCallbackID() string {
	return i.pickerCB
}

// ShowPicker displays the picker with items.
func (i *Input) ShowPicker(opts ui.ShowPickerMsg) {
	i.picker.SetItems(opts.Items)
	i.pickerCB = opts.CallbackID
	i.pickerActive = true
	i.pickerDismiss = opts.DismissOnSpace

	if opts.Inline {
		i.picker.SetHeader("")
		i.picker.Filter(i.textinput.Value())
	} else {
		header := opts.Title
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

// UpdatePickerFilter updates filter based on input value. Closing the
// picker when the input empties (or hits a space, for dismiss-on-space
// pickers) is the model's job - it must also reset the input mode and
// cancel the Lua callback.
func (i *Input) UpdatePickerFilter() {
	i.picker.Filter(i.textinput.Value())
}

// PickerDismissOnSpace reports whether the active picker asked to be
// closed once the input contains a space.
func (i *Input) PickerDismissOnSpace() bool {
	return i.pickerDismiss
}
