package widget

import (
	"image"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
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
	textinput textinput.Model
	composer  *composer
	picker    *Picker
	search    *Search
	styles    style.Styles

	// State
	overlay        inputOverlay
	discardPending bool
	selected       bool // whole line selected (keep-input resend state)
	width          int
	height         int
}

// NewInput creates the input widget with its scrollback-search child. Search
// is required because the widget delegates its entire surface to that child
// while search mode is active.
func NewInput(styles style.Styles, search *Search) *Input {
	if search == nil {
		panic("widget.NewInput requires a search widget")
	}
	ti := textinput.New()
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
		if i.composer != nil {
			i.UpdateComposer(key)
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
	if key.Text != "" || matchesKey(key, tea.KeyBackspace, 0) || matchesKey(key, tea.KeyDelete, 0) {
		i.textinput.SetValue("")
		i.textinput.SetCursor(0)
	}
	i.Deselect()
}

// SelectAll marks the whole line selected: the keep-input resend state,
// where Enter resends it and typing replaces it. No-op while composing
// or empty.
func (i *Input) SelectAll() {
	if i.composer != nil || i.textinput.Value() == "" {
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

// View implements Widget.
func (i *Input) View() string {
	// Search is a modal navigator, not an inline completion surface. It
	// replaces the command field while active so the terminal never shows
	// two apparent cursors competing for keyboard focus.
	if i.SearchActive() {
		if i.height > 0 && !i.search.frameFits(i.width, i.height) {
			return i.search.constrainedView(i.height)
		}
		return i.search.contentView()
	}

	plan := i.layout(i.width, i.height)
	rows := make([]string, plan.height)
	if plan.pickerHeight > 0 {
		copy(rows[:plan.pickerHeight], strings.Split(i.picker.contentView(plan.pickerHeight), "\n"))
	}
	if i.composer != nil {
		copy(rows[plan.body.Min.Y:plan.body.Max.Y], i.composerRows(plan.body.Dy()))
	} else if !plan.body.Empty() {
		// Keep the ordinary one-line input in its compact three-row layout.
		// Compose chrome exists only around structured text.
		inputView := i.textinput.View()
		if i.selected {
			// Bubbles renders TextStyle across its width padding. Render the
			// selected value without that padding, then fill the row normally so
			// only actual command text receives the selection background.
			selectedInput := i.textinput
			selectedInput.SetWidth(0)
			selectedInput.Blur() // the selection replaces the visual caret
			inputView = selectedInput.View()
			if padding := i.width - util.VisibleLen(inputView); padding > 0 {
				inputView += strings.Repeat(" ", padding)
			}
		}
		rows[plan.body.Min.Y] = inputView
	}
	// Decorations are painted once by the compositor, after all content.
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
	i.picker.SetWidth(width)
	i.search.SetWidth(width)
	if i.composer != nil && !i.SearchActive() {
		layout := buildComposerLayout(i.composer.text, i.composer.cursor, width)
		i.composer.topRow = i.composerTopRow(layout, i.layout(width, height).body.Dy())
	}
}

func (i *Input) MinimumSize() image.Point {
	if i.SearchActive() {
		chrome := i.search.styles.OverlayBorder.GetHorizontalBorderSize() + i.search.styles.OverlayBorder.GetHorizontalPadding()
		return image.Pt(chrome+1, i.search.PreferredHeight())
	}
	return image.Pt(3, 3)
}

// MeasureHeight does not resize the editor or its children.
func (i *Input) MeasureHeight(width, limit int) int {
	if i.SearchActive() {
		return min(limit, i.search.PreferredHeight())
	}

	h := 3 // normal: top border + input + bottom border
	if i.composer != nil {
		layout := buildComposerLayout(i.composer.text, i.composer.cursor, width)
		bodyRows := clampInt(len(layout.rows), 1, maxComposerBodyRows)
		h = bodyRows + 2 // status header + content + key footer
	}
	if i.PickerActive() {
		h += i.picker.PreferredHeight()
	}
	return min(h, limit)
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
	i.Deselect()
	if i.composer != nil {
		// Verbatim interpretation is sticky: replacing a structured draft
		// with one non-empty physical line (for example through Ctrl+E) must
		// not silently re-enable command-separator or slash-command processing.
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
	i.Deselect()
	if i.composer != nil {
		i.composer.SetCursor(pos)
		return
	}
	i.textinput.SetCursor(pos)
}

// Reset clears the input.
func (i *Input) Reset() {
	i.composer = nil
	i.Deselect()
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
	i.Deselect()
	i.composer = newComposer(text, cursor)
	i.discardPending = false
}

// InsertPaste inserts one atomic bracketed-paste payload. Safe, plain
// single-line pastes retain the existing textinput UX; structured or
// terminal-active content switches in place at the current cursor without
// losing the already-typed prefix or suffix.
func (i *Input) InsertPaste(text string) tea.Cmd {
	if i.selected {
		// Pasting over a selection replaces it, like typing.
		i.textinput.SetValue("")
		i.textinput.SetCursor(0)
		i.Deselect()
	}
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
	msg := tea.PasteMsg{Content: text}
	i.textinput, cmd = i.textinput.Update(msg)
	return cmd
}

// UpdateComposer applies local editing/navigation keys. The return value is
// false for keys owned by the controller (notably plain Enter, Escape,
// Ctrl+C, and Ctrl+E). Compose mode remains sticky until submit/cancel so an
// edit can never silently change the draft's interpretation.
func (i *Input) UpdateComposer(msg tea.KeyPressMsg) bool {
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
		i.picker.queryVisible = true
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
