package session

import (
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

// Print implements lua.Host. Scripts routinely re-print captured
// server text, so display sanitization applies here too (issue #69);
// rune.style output is SGR and passes through untouched.
func (s *Session) Print(msg string) {
	s.ui.Print(text.SanitizeDisplay(msg))
}

// PaneCreate implements lua.Host.
func (s *Session) PaneCreate(name string) {
	s.ui.CreatePane(name)
}

// PaneWrite implements lua.Host. Sanitized like Print: pane content is
// often trigger-captured server text.
func (s *Session) PaneWrite(name, msg string) {
	s.ui.WritePane(name, text.SanitizeDisplay(msg))
}

// PaneToggle implements lua.Host.
func (s *Session) PaneToggle(name string) {
	s.ui.TogglePane(name)
}

// PaneSetVisible implements lua.Host.
func (s *Session) PaneSetVisible(name string, visible bool) {
	s.ui.SetPaneVisible(name, visible)
}

// PaneClear implements lua.Host.
func (s *Session) PaneClear(name string) {
	s.ui.ClearPane(name)
}

// ClipboardSet implements lua.Host.
func (s *Session) ClipboardSet(text string) {
	s.ui.SetClipboard(text)
}

// ShowPicker implements lua.Host.
func (s *Session) ShowPicker(opts ui.ShowPickerMsg) {
	s.ui.ShowPicker(opts)
}

// GetInput implements lua.Host.
func (s *Session) GetInput() string {
	return s.currentInput
}

// SetInput implements lua.Host.
func (s *Session) SetInput(text string) {
	s.ui.SetInput(text)
	s.currentInput = text
	s.currentCursor = len(text)
}

// SetInputSubmission implements lua.Host. History recall uses it to restore
// interpretation as well as text, including one-line verbatim drafts that
// would otherwise look like ordinary command input.
func (s *Session) SetInputSubmission(submission input.Submission) {
	s.ui.SetInputSubmission(submission)
	s.currentInput = submission.Text
	s.currentCursor = len(submission.Text)
}

// InputGetCursor implements lua.Host.
func (s *Session) InputGetCursor() int {
	return s.currentCursor
}

// InputSetCursor implements lua.Host. Lua supplies a UTF-8 byte offset;
// the input widget expects a rune offset.
func (s *Session) InputSetCursor(pos int) {
	pos = input.ClampByteCursor(s.currentInput, pos)
	s.currentCursor = pos
	s.ui.InputSetCursor(input.ByteCursorToRune(s.currentInput, pos))
}

// OpenEditor implements lua.Host.
func (s *Session) OpenEditor(initial string) (string, bool) {
	return s.ui.OpenEditor(initial)
}

// PaneScrollUp implements lua.Host.
func (s *Session) PaneScrollUp(name string, lines int) {
	s.ui.PaneScrollUp(name, lines)
}

// PaneScrollDown implements lua.Host.
func (s *Session) PaneScrollDown(name string, lines int) {
	s.ui.PaneScrollDown(name, lines)
}

// PaneScrollToTop implements lua.Host.
func (s *Session) PaneScrollToTop(name string) {
	s.ui.PaneScrollToTop(name)
}

// PaneScrollToBottom implements lua.Host.
func (s *Session) PaneScrollToBottom(name string) {
	s.ui.PaneScrollToBottom(name)
}

// handlePickerResult delegates to Engine for callback execution or cancellation.
func (s *Session) handlePickerResult(id string, value string, accepted bool) {
	if accepted {
		s.engine.ExecutePickerCallback(id, value)
	} else {
		s.engine.CancelPickerCallback(id)
	}
}
