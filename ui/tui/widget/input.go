package widget

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// Compile-time check that Input implements Widget
var _ Widget = (*Input)(nil)

// Input handles the input area including text entry, picker overlay, and borders.
type Input struct {
	textinput textinput.Model
	composer  *Composer
	picker    *Picker
	styles    style.Styles

	// State
	pickerActive   bool
	discardPending bool
	width          int
	height         int
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
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.Paste {
			return i.InsertPaste(string(key.Runes))
		}
		if i.composer != nil {
			i.UpdateComposer(key)
			return nil
		}
	}

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

	if i.composer != nil {
		parts = append(parts, i.composerView()...)
	} else {
		// Keep the ordinary one-line input byte-for-byte identical to the
		// original widget. Compose chrome exists only around structured text.
		parts = append(parts, i.borderLine())
		parts = append(parts, i.textinput.View())
		parts = append(parts, i.borderLine())
	}

	return strings.Join(parts, "\n")
}

// SetSize implements Widget.
func (i *Input) SetSize(width, height int) {
	i.width = width
	i.height = height
	i.textinput.Width = width - 2 // Account for prompt
	i.picker.SetWidth(width)
}

// PreferredHeight implements Widget.
func (i *Input) PreferredHeight() int {
	h := 3 // normal: top border + input + bottom border
	if i.composer != nil {
		layout := buildComposerLayout(i.composer.text, i.composer.cursor, i.width)
		bodyRows := clampInt(len(layout.rows), 1, maxComposerBodyRows)
		h = bodyRows + 2 // status header + content + key footer
	}
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
	if i.composer != nil {
		return i.composer.Value()
	}
	return i.textinput.Value()
}

// SetValue sets the input text.
func (i *Input) SetValue(s string) {
	if i.composer != nil {
		// Verbatim interpretation is sticky: replacing a structured draft
		// with one non-empty physical line (for example through Ctrl+E) must
		// not silently re-enable delimiter or slash-command processing.
		if s == "" {
			i.Reset()
			return
		}
		i.composer.Set(s, len([]rune(normalizeComposerText(s))))
		i.discardPending = false
		return
	}
	if RequiresComposer(s) {
		i.BeginCompose(s, len([]rune(normalizeComposerText(s))))
		return
	}
	i.textinput.SetValue(s)
}

// CursorEnd moves the cursor to the end.
func (i *Input) CursorEnd() {
	if i.composer != nil {
		i.composer.CursorEnd()
		return
	}
	i.textinput.CursorEnd()
}

// Position returns the cursor position.
func (i *Input) Position() int {
	if i.composer != nil {
		return i.composer.Position()
	}
	return i.textinput.Position()
}

// SetCursor sets the cursor position.
func (i *Input) SetCursor(pos int) {
	if i.composer != nil {
		i.composer.SetCursor(pos)
		return
	}
	i.textinput.SetCursor(pos)
}

// Reset clears the input.
func (i *Input) Reset() {
	if i.composer != nil {
		i.composer.Reset()
		i.composer = nil
	}
	i.discardPending = false
	i.textinput.SetValue("")
	i.textinput.SetCursor(0)
}

// IsComposing reports whether the lossless structured-text editor is active.
func (i *Input) IsComposing() bool {
	return i.composer != nil
}

// BeginCompose replaces the active input with a canonical structured draft.
// It does not submit and it never routes the text through bubbles/textinput.
func (i *Input) BeginCompose(text string, cursor int) {
	i.composer = newComposer(text, cursor)
	i.discardPending = false
}

// EndCompose migrates a now-plain draft back into the ordinary textinput.
// A caller cannot accidentally collapse LF/TAB content into a widget that
// would render or sanitize it incorrectly.
func (i *Input) EndCompose() bool {
	if i.composer == nil {
		return true
	}
	value := i.composer.Value()
	if RequiresComposer(value) {
		return false
	}
	pos := i.composer.Position()
	i.composer = nil
	i.discardPending = false
	i.textinput.SetValue(value)
	i.textinput.SetCursor(pos)
	return true
}

// InsertPaste inserts one atomic bracketed-paste payload. Safe, plain
// single-line pastes retain the existing textinput UX; structured or
// terminal-active content switches in place at the current cursor without
// losing the already-typed prefix or suffix.
func (i *Input) InsertPaste(text string) tea.Cmd {
	i.discardPending = false
	text = normalizeComposerText(text)
	if i.composer != nil {
		i.composer.Insert(text)
		return nil
	}
	if RequiresComposer(text) {
		value := i.textinput.Value()
		cursor := i.textinput.Position()
		i.BeginCompose(value, cursor)
		i.composer.Insert(text)
		return nil
	}

	var cmd tea.Cmd
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true}
	i.textinput, cmd = i.textinput.Update(msg)
	return cmd
}

// UpdateComposer applies local editing/navigation keys. The return value is
// false for keys owned by the controller (notably plain Enter, Escape,
// Ctrl+C, and Ctrl+E). Compose mode remains sticky until submit/cancel so an
// edit can never silently change the draft's interpretation.
func (i *Input) UpdateComposer(msg tea.KeyMsg) bool {
	if i.composer == nil {
		return false
	}
	handled := i.composer.Update(msg, i.width)
	if handled {
		i.discardPending = false
	}
	return handled
}

// ConfirmDiscard arms the first Escape and reports true only on the second.
// Large composed drafts should never disappear from one accidental keypress.
func (i *Input) ConfirmDiscard() bool {
	if i.discardPending {
		return true
	}
	i.discardPending = true
	return false
}

// ContinueCompose dismisses a pending discard confirmation.
func (i *Input) ContinueCompose() {
	i.discardPending = false
}

// CanMoveComposerVertically reports whether a one-row vertical move would
// remain inside the current visual document. Controllers use the boundary to
// hand unmodified recalled entries back to Lua history navigation.
func (i *Input) CanMoveComposerVertically(delta int) bool {
	if i.composer == nil || delta == 0 {
		return false
	}
	layout := buildComposerLayout(i.composer.text, i.composer.cursor, i.width)
	if delta < 0 {
		return layout.cursorRow > 0
	}
	return layout.cursorRow < len(layout.rows)-1
}

func (i *Input) composerView() []string {
	layout := buildComposerLayout(i.composer.text, i.composer.cursor, i.width)
	bodyHeight := clampInt(len(layout.rows), 1, maxComposerBodyRows)
	if i.height > 0 {
		// The default layout gives us PreferredHeight. An explicit Lua layout
		// height is also honored so View emits exactly the rows it was allotted.
		bodyHeight = max(1, i.height-2)
	}

	maxTop := max(0, len(layout.rows)-bodyHeight)
	i.composer.topRow = clampInt(i.composer.topRow, 0, maxTop)
	if layout.cursorRow < i.composer.topRow {
		i.composer.topRow = layout.cursorRow
	} else if layout.cursorRow >= i.composer.topRow+bodyHeight {
		i.composer.topRow = layout.cursorRow - bodyHeight + 1
	}
	i.composer.topRow = clampInt(i.composer.topRow, 0, maxTop)

	lineWord := "lines"
	if layout.lineCount == 1 {
		lineWord = "line"
	}
	header := i.composeHeader(fmt.Sprintf("VERBATIM · %d %s", layout.lineCount, lineWord))
	rows := []string{header}

	for n := 0; n < bodyHeight; n++ {
		rowIndex := i.composer.topRow + n
		if rowIndex >= len(layout.rows) {
			rows = append(rows, strings.Repeat(" ", max(0, i.width)))
			continue
		}
		rows = append(rows, i.renderComposerRow(layout, rowIndex))
	}

	help := "Enter verbatim · Ctrl+Enter newline · Esc discard"
	if i.discardPending {
		help = "Esc again discard · Any other key keep editing"
	}
	rows = append(rows, i.composeFooter(help))
	return rows
}

func (i *Input) renderComposerRow(layout composerLayout, rowIndex int) string {
	row := layout.rows[rowIndex]
	var b strings.Builder

	if layout.gutterSize > 0 {
		digits := layout.gutterSize - 3
		if row.continuation {
			b.WriteString(i.styles.Muted.Render(strings.Repeat(" ", digits) + " ↳ "))
		} else {
			b.WriteString(i.styles.Muted.Render(fmt.Sprintf("%*d │ ", digits, row.line+1)))
		}
	}

	col := 0
	cursorDrawn := false
	for _, glyph := range row.glyphs {
		if rowIndex == layout.cursorRow && col == layout.cursorCol && !cursorDrawn {
			b.WriteString(i.styles.InputCursor.Render(glyph.text))
			cursorDrawn = true
		} else {
			b.WriteString(i.styles.InputText.Render(glyph.text))
		}
		col += glyph.width
	}
	if rowIndex == layout.cursorRow && !cursorDrawn {
		b.WriteString(i.styles.InputCursor.Render(" "))
	}

	view := b.String()
	if padding := i.width - util.VisibleLen(view); padding > 0 {
		view += strings.Repeat(" ", padding)
	}
	return clipRow(view, i.width)
}

func (i *Input) composeHeader(label string) string {
	if i.width < 1 {
		return ""
	}
	label = " " + label + " "
	if util.VisibleLen(label) >= i.width {
		return i.styles.Warning.Render(clipRow(label, i.width))
	}
	fill := strings.Repeat("─", i.width-util.VisibleLen(label))
	return i.styles.Muted.Render(fill) + i.styles.Warning.Render(label)
}

func (i *Input) composeFooter(help string) string {
	if i.width < 1 {
		return ""
	}
	label := " " + help + " "
	if util.VisibleLen(label) >= i.width {
		return i.styles.Muted.Render(clipRow(label, i.width))
	}
	fill := strings.Repeat("─", i.width-util.VisibleLen(label))
	return i.styles.Muted.Render(label + fill)
}

// Picker access

// ShowPicker displays the picker with items. The picker's session-side
// state (callback ID, dismiss-on-space) is owned by the input
// controller; the widget only renders the overlay.
func (i *Input) ShowPicker(opts ui.ShowPickerMsg) {
	i.picker.SetItems(opts.Items)
	i.pickerActive = true

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
// pickers) is the input controller's job - it must also reset the
// input mode and cancel the Lua callback.
func (i *Input) UpdatePickerFilter() {
	i.picker.Filter(i.textinput.Value())
}
