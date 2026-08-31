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
	SetInput(text string)
	SetInputSubmission(submission input.Submission)

	// Updates
	UpdateBars(content map[string]BarContent)
	UpdateBinds(keys map[string]bool)
	UpdateLayout(layout LayoutTree)
	UpdateConfig(cfg Config)

	// Components
	ShowPicker(opts ShowPickerMsg)
	ShowSearch(opts ShowSearchMsg)
	SetClipboard(text string)
	CreatePane(name string)
	WritePane(name, text string)
	ClearPane(name string)

	// Input primitives. Cursor positions are zero-based rune offsets.
	InputSetCursor(pos int)
	OpenEditor(initial string) (string, bool)

	// Pane scrolling primitives for Lua
	PaneScrollUp(name string, lines int)
	PaneScrollDown(name string, lines int)
	PaneScrollToTop(name string)
	PaneScrollToBottom(name string)
}
