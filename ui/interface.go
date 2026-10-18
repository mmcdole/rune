package ui

import "github.com/mmcdole/rune/input"

// UI is the Session-facing display and input contract.
type UI interface {
	Run() error
	Quit()

	// Input/Output
	Events() <-chan UIEvent
	Print(text string)
	Echo(text string)
	SetPrompt(text string)
	CommitPrompt(text string)
	// Draft edits carry Session's revision for that edit. The UI echoes the
	// latest revision it has applied in InputChangedMsg, DraftAppliedMsg, and
	// CursorMovedMsg.
	SetInput(text string, rev uint64)
	SetInputSubmission(submission input.Submission, rev uint64)

	// Updates
	UpdateBars(content map[string]BarContent)
	UpdateBinds(keys input.Bindings)
	// UpdateLayout takes a tree produced by NormalizeLayoutTree: valid, with
	// every child's size resolved.
	UpdateLayout(layout LayoutTree)
	UpdateConfig(cfg Config)

	// Components
	ShowPicker(opts PickerOptions)
	ShowSearch(opts SearchOptions)
	SetClipboard(text string)
	WritePane(name, text string)
	ReplacePane(name, text string)
	ClearPane(name string)

	// Input primitives. Cursor positions are zero-based rune offsets.
	InputSetCursor(pos int, rev uint64)
	OpenEditor(initial string) (string, bool)

	// Pane scrolling primitives for Lua
	PaneScrollUp(name string, lines int)
	PaneScrollDown(name string, lines int)
	PaneScrollToTop(name string)
	PaneScrollToBottom(name string)
}
