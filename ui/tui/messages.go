package tui

import (
	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

// printLineMsg carries text to append to scrollback. It may contain newlines;
// the TUI splits and wraps it into rows.
type printLineMsg string

// echoLineMsg carries a local echo (submitted input, already styled by the
// Lua "echo" hook) to append to scrollback. Like printLineMsg, the
// text may contain newlines.
type echoLineMsg string

// setPromptMsg replaces the prompt overlay with a partial line or confirmed prompt.
type setPromptMsg string

// commitPromptMsg moves the prompt overlay to scrollback in one update.
type commitPromptMsg string

// paneWriteMsg appends a line to a named pane.
type paneWriteMsg struct {
	Name string
	Text string
}

// paneReplaceMsg empties a named pane and writes Text in one update, so a
// redrawn status pane never renders an intermediate empty frame.
type paneReplaceMsg struct {
	Name string
	Text string
}

// paneCreateMsg creates a new named pane.
type paneCreateMsg struct {
	Name string
}

// paneClearMsg clears the contents of a named pane.
type paneClearMsg struct {
	Name string
}

// updateBindsMsg replaces the UI snapshot used for action routing and hints.
type updateBindsMsg input.Bindings

// updateBarsMsg pushes rendered bar content from Session to UI.
// Session runs Lua bar renderers and sends the result; UI just displays it.
type updateBarsMsg map[string]ui.BarContent

// updateLayoutMsg pushes the canonical layout tree from Session to UI.
type updateLayoutMsg ui.LayoutTree

// updateConfigMsg pushes UI-facing configuration from Session to UI.
type updateConfigMsg ui.Config

// setClipboardMsg asks the terminal to set the system clipboard
// (OSC 52). Sent from Session when Lua calls rune.clipboard.set().
type setClipboardMsg string

// setInputMsg sets the input line content.
// Sent from Session when Lua calls rune.input.set().
type setInputMsg string

// setInputSubmissionMsg restores both draft text and interpretation.
// History recall uses this for one-line verbatim entries that have no
// structural character from which the UI could infer draft editor mode.
type setInputSubmissionMsg input.Submission

// inputSetCursorMsg sets the widget cursor to a zero-based rune offset.
type inputSetCursorMsg int

// paneScrollUpMsg scrolls a pane up by N lines.
type paneScrollUpMsg struct {
	Name  string
	Lines int
}

// paneScrollDownMsg scrolls a pane down by N lines.
type paneScrollDownMsg struct {
	Name  string
	Lines int
}

// paneScrollToTopMsg scrolls a pane to the top.
type paneScrollToTopMsg struct {
	Name string
}

// paneScrollToBottomMsg scrolls a pane to the bottom.
type paneScrollToBottomMsg struct {
	Name string
}

// showPickerMsg opens a picker on the model goroutine.
type showPickerMsg struct{ options ui.PickerOptions }

// showSearchMsg opens search on the model goroutine.
type showSearchMsg struct{ options ui.SearchOptions }
