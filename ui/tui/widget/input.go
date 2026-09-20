package widget

import (
	"image"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
)

// Compile-time check that Input implements Widget
var _ Widget = (*Input)(nil)

type inputOverlay uint8

const (
	overlayNone inputOverlay = iota
	overlayPickerModal
	overlayPickerInline
	overlaySearch
)

// Input handles the input area including text entry, picker overlay, and borders.
type Input struct {
	keys        input.Bindings
	textinput   textinput.Model
	draftEditor *draftEditor
	picker      *Picker
	search      *Search
	styles      style.Styles

	// State
	submissionMode input.SubmissionMode
	modeExplicit   bool // user choice or history restoration, retained for this draft
	overlay        inputOverlay
	discardPending bool
	selected       bool // whole line selected (keep-input resend state)
	width          int
	height         int
}

// NewInput creates the input widget. Search supplies scrollback matches and
// navigation state; Input owns rendering, measurement, and decoration.
func NewInput(styles style.Styles, search *Search) *Input {
	if search == nil {
		panic("widget.NewInput requires a search widget")
	}
	ti := textinput.New()
	// Enhanced keyboard reporting distinguishes Backspace held with Shift.
	ti.KeyMap.DeleteCharacterBackward.SetKeys(append(ti.KeyMap.DeleteCharacterBackward.Keys(), "shift+backspace")...)
	ti.Placeholder = ""
	ti.Prompt = "> "
	ti.CharLimit = 0
	ti.SetWidth(80)
	textStyles := ti.Styles()
	textStyles.Focused.Text = styles.InputText
	textStyles.Blurred.Text = styles.InputText
	textStyles.Focused.Prompt = styles.InputText
	textStyles.Blurred.Prompt = styles.InputText
	textStyles.Cursor.Color = styles.InputCursor.GetBackground()
	ti.SetStyles(textStyles)
	ti.Focus()

	return &Input{
		keys:      input.DefaultBindings(),
		textinput: ti,
		picker: NewPicker(PickerConfig{
			MaxVisible: 10,
			EmptyText:  "No matches",
		}, styles),
		search: search,
		styles: styles,
	}
}

// UpdateTextInput forwards messages to the underlying textinput.
func (i *Input) UpdateTextInput(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if i.draftEditor != nil {
			i.UpdateDraftEditor(key)
			return nil
		}
		if i.selected {
			i.resolveSelection(key)
		}
	}

	var cmd tea.Cmd
	i.textinput, cmd = i.textinput.Update(msg)
	return cmd
}

// resolveSelection applies select-and-replace semantics before an
// editing key reaches the textinput: typing or deleting replaces the
// whole selected line, any other key deselects and edits in place.
func (i *Input) resolveSelection(key tea.KeyPressMsg) {
	if key.Text != "" || matchesKey(key, tea.KeyBackspace, 0) ||
		matchesKey(key, tea.KeyBackspace, tea.ModShift) || matchesKey(key, tea.KeyDelete, 0) {
		i.Reset()
	}
	i.Deselect()
}

// SelectAll marks the whole draft selected: Enter resends it and typing
// replaces it. Empty drafts have no selection.
func (i *Input) SelectAll() {
	if i.Value() == "" {
		return
	}
	i.selected = true
	styles := i.textinput.Styles()
	styles.Focused.Text = i.styles.InputSelected
	styles.Blurred.Text = i.styles.InputSelected
	i.textinput.SetStyles(styles)
}

// Deselect leaves the selected state, keeping the text editable.
func (i *Input) Deselect() {
	if !i.selected {
		return
	}
	i.selected = false
	styles := i.textinput.Styles()
	styles.Focused.Text = i.styles.InputText
	styles.Blurred.Text = i.styles.InputText
	i.textinput.SetStyles(styles)
}

// Selected reports whether the whole line is selected.
func (i *Input) Selected() bool {
	return i.selected
}

// View draws within the allocated size; unallocated input has no content.
func (i *Input) View() string {
	if i.width <= 0 || i.height <= 0 {
		return ""
	}
	plan := i.layout(i.width, i.height)
	rows := make([]string, i.height)
	if i.SearchActive() {
		if !plan.results.Empty() {
			copy(rows[plan.results.Min.Y:plan.results.Max.Y], i.search.resultLines(plan.results.Dx(), plan.results.Dy()))
		}
		if plan.help >= 0 {
			rows[plan.help] = i.search.footerLine(i.width, i.keys.Hint("cancel"))
		}
		if !plan.body.Empty() {
			rows[plan.body.Min.Y] = i.search.queryLine(plan.body.Dx())
		}
		return strings.Join(rows, "\n")
	}
	if !plan.results.Empty() {
		copy(rows[plan.results.Min.Y:plan.results.Max.Y], i.picker.resultRows(plan.results.Dx(), plan.results.Dy()))
	}
	if i.PickerActive() && !i.PickerInline() {
		if !plan.body.Empty() {
			rows[plan.body.Min.Y] = i.picker.queryLine(plan.body.Dx())
		}
		return strings.Join(rows, "\n")
	}
	if i.draftEditor != nil {
		copy(rows[plan.body.Min.Y:plan.body.Max.Y], i.draftRows(plan.body.Dy()))
	} else if !plan.body.Empty() {
		// Keep the ordinary one-line input in its compact three-row layout.
		// Mode labels are rendered on the surrounding rules.
		inputView := i.textinput.View()
		if i.selected {
			// Bubbles renders TextStyle across its width padding. Render the
			// selected value without that padding, then fill the row normally so
			// only actual command text receives the selection background.
			selectedInput := i.textinput
			selectedInput.SetWidth(0)
			selectedInput.Blur() // the selection replaces the visual caret
			inputView = selectedInput.View()
			if padding := i.width - ansi.StringWidth(inputView); padding > 0 {
				inputView += strings.Repeat(" ", padding)
			}
		}
		rows[plan.body.Min.Y] = inputView
	}
	// Decorations are rendered once by the renderer, after all content.
	return strings.Join(rows, "\n")
}

// SetSize implements Widget.
func (i *Input) SetSize(width, height int) {
	i.width = width
	i.height = height
	i.textinput.Prompt = "> "
	if width < 3 {
		i.textinput.Prompt = ""
	}
	i.textinput.SetWidth(max(0, width-len(i.textinput.Prompt)))
	if width > 0 && height > 0 && i.draftEditor != nil && !i.SearchActive() {
		layout := i.draftEditor.layout(width)
		i.draftEditor.topRow = i.draftTopRow(layout, i.layout(width, height).body.Dy())
	}
}

func (i *Input) MinimumSize() image.Point {
	return image.Pt(3, 3)
}

// MeasureHeight does not resize the draft editor or its children.
func (i *Input) MeasureHeight(width, limit int) int {
	if i.SearchActive() {
		// Matching rows, one help row, one query row, and three separators.
		return min(limit, i.search.resultHeight()+5)
	}
	if i.PickerActive() && !i.PickerInline() {
		return min(limit, i.picker.resultHeight()+4)
	}

	h := 3 // normal: top border + input + bottom border
	if i.draftEditor != nil {
		bodyRows := i.draftEditor.measureRows(width)
		h = bodyRows + 2 // status header + content + key footer
	}
	if i.PickerActive() {
		h += i.picker.resultHeight() + 1
	}
	return min(h, limit)
}

// Value returns the current input text.
func (i *Input) Value() string {
	if i.draftEditor != nil {
		return i.draftEditor.Value()
	}
	return i.textinput.Value()
}

// SetValue sets the input text.
func (i *Input) SetValue(s string) {
	if s == "" {
		i.Reset()
		return
	}
	i.Deselect()
	if i.draftEditor != nil {
		// Preserve the draft editor and interpretation when an edit
		// replaces the draft with one non-empty physical line.
		i.draftEditor.Set(s, len([]rune(input.NormalizeDraftText(s))))
		i.discardPending = false
		return
	}
	if input.RequiresStructuredEditor(s) {
		i.OpenDraftEditor(s, len([]rune(input.NormalizeDraftText(s))))
		return
	}
	i.textinput.SetValue(s)
}

// CursorEnd moves the cursor to the end.
func (i *Input) CursorEnd() {
	if i.draftEditor != nil {
		i.draftEditor.CursorEnd()
		return
	}
	i.textinput.CursorEnd()
}

// Position returns the cursor position.
func (i *Input) Position() int {
	if i.draftEditor != nil {
		return i.draftEditor.Position()
	}
	return i.textinput.Position()
}

// SetCursor sets the cursor position.
func (i *Input) SetCursor(pos int) {
	i.Deselect()
	if i.draftEditor != nil {
		i.draftEditor.SetCursor(pos)
		return
	}
	i.textinput.SetCursor(pos)
}

// Reset clears the input.
func (i *Input) Reset() {
	i.submissionMode = input.ModeCommand
	i.modeExplicit = false
	i.draftEditor = nil
	i.Deselect()
	i.discardPending = false
	i.textinput.SetValue("")
	i.textinput.SetCursor(0)
}

// SubmissionMode is the draft's interpretation, independent of its editor.
func (i *Input) SubmissionMode() input.SubmissionMode { return i.submissionMode }

// SetSubmissionMode makes an explicit choice without changing text, cursor,
// selection, or the draft editor. Structured pastes respect this choice.
func (i *Input) SetSubmissionMode(mode input.SubmissionMode) {
	i.submissionMode = mode
	i.modeExplicit = true
	i.discardPending = false
}

// ToggleSubmissionMode opens the draft editor when necessary so an explicit mode
// change is always visible, preserving text, cursor, and selection.
func (i *Input) ToggleSubmissionMode() {
	mode := input.ModeVerbatim
	if i.submissionMode == input.ModeVerbatim {
		mode = input.ModeCommand
	}
	i.SetSubmissionMode(mode)
	if i.draftEditor == nil {
		i.draftEditor = newDraftEditor(i.textinput.Value(), i.textinput.Position())
	}
}

// DraftEditorActive reports whether the lossless draft editor is active.
func (i *Input) DraftEditorActive() bool {
	return i.draftEditor != nil
}

// OpenDraftEditor replaces the active input with a canonical structured draft.
// It does not submit and it never routes the text through bubbles/textinput.
func (i *Input) OpenDraftEditor(text string, cursor int) {
	if !i.modeExplicit {
		i.submissionMode = input.ModeVerbatim
	}
	i.Deselect()
	i.draftEditor = newDraftEditor(text, cursor)
	i.discardPending = false
}

// InsertPaste inserts one atomic bracketed-paste payload. Safe, plain
// single-line pastes retain the existing textinput UX; structured or
// terminal-active content switches in place at the current cursor without
// losing the already-typed prefix or suffix.
func (i *Input) InsertPaste(text string) tea.Cmd {
	if i.selected {
		// Pasting over a selection replaces it, like typing.
		i.Reset()
		i.Deselect()
	}
	i.discardPending = false
	text = input.NormalizeDraftText(text)
	if i.draftEditor != nil {
		i.draftEditor.Insert(text)
		return nil
	}
	if input.RequiresStructuredEditor(text) {
		value := i.textinput.Value()
		cursor := i.textinput.Position()
		i.OpenDraftEditor(value, cursor)
		i.draftEditor.Insert(text)
		return nil
	}

	var cmd tea.Cmd
	msg := tea.PasteMsg{Content: text}
	i.textinput, cmd = i.textinput.Update(msg)
	return cmd
}

// UpdateDraftEditor applies local editing/navigation keys. The return value is
// false for keys owned by the controller (notably plain Enter, Escape,
// Ctrl+C, and Ctrl+E). Draft editor mode remains sticky until submit/cancel so an
// edit can never silently change the draft's interpretation.
func (i *Input) UpdateDraftEditor(msg tea.KeyPressMsg) bool {
	if i.draftEditor == nil {
		return false
	}
	if i.selected {
		i.resolveSelection(msg)
		if i.draftEditor == nil {
			// Typing or deleting over the selection starts a fresh draft.
			i.UpdateTextInput(msg)
			return true
		}
	}
	handled := i.draftEditor.Update(msg, i.width)
	if handled {
		i.discardPending = false
	}
	return handled
}

// ConfirmDiscard arms the first cancel action and confirms on the second.
// Large drafts should never disappear from one accidental keypress.
func (i *Input) ConfirmDiscard() bool {
	if i.discardPending {
		return true
	}
	i.discardPending = true
	return false
}

// ContinueEditing dismisses a pending discard confirmation.
func (i *Input) ContinueEditing() {
	i.discardPending = false
}

// CanMoveDraftEditorVertically reports whether a one-row vertical move would
// remain inside the current visual document. Controllers use the boundary to
// hand unmodified recalled entries back to Lua history navigation.
func (i *Input) CanMoveDraftEditorVertically(delta int) bool {
	if i.draftEditor == nil || delta == 0 {
		return false
	}
	layout := i.draftEditor.layout(i.width)
	if delta < 0 {
		return layout.cursorRow > 0
	}
	return layout.cursorRow < len(layout.rows)-1
}

// Picker access

// ShowPicker displays the picker with items. The picker's session-side
// state (callback ID, dismiss-on-space) is owned by the input
// controller; the widget only renders the overlay.
func (i *Input) ShowPicker(opts ui.ShowPickerMsg) {
	i.picker.SetItems(opts.Items)
	i.overlay = overlayPickerModal

	if opts.Inline {
		i.overlay = overlayPickerInline
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
	i.overlay = overlayNone
	i.picker.Reset()
}

// Search access

// ShowSearch opens the search overlay. An empty query keeps the
// previous search's query (the widget persists across open/close).
func (i *Input) ShowSearch(query string, scope SearchScope) {
	i.search.Open(query, scope)
	i.overlay = overlaySearch
}

// HideSearch closes the search overlay. Query and match state persist
// in the widget for the next ShowSearch.
func (i *Input) HideSearch() {
	i.overlay = overlayNone
}

// SearchActive reports whether the search overlay is showing.
func (i *Input) SearchActive() bool {
	return i.overlay == overlaySearch
}

func (i *Input) PickerActive() bool {
	return i.overlay == overlayPickerModal || i.overlay == overlayPickerInline
}

func (i *Input) PickerInline() bool { return i.overlay == overlayPickerInline }

// Picker exposes local query and selection operations; show/hide transitions
// stay on Input so focus and geometry always agree.
func (i *Input) Picker() *Picker { return i.picker }

func (i *Input) Search() *Search { return i.search }

// SetBindings installs a registry snapshot used for matching and hints.
func (i *Input) SetBindings(keys input.Bindings) {
	i.keys = keys
	i.discardPending = false
}
func (i *Input) Bindings() input.Bindings { return i.keys }
