package ui

import "github.com/mmcdole/rune/input"

// UI defines the contract for the terminal display layer.
// Implementation lives in the same package (BubbleTeaUI).
type UI interface {
	Run() error
	Quit()

	// Input/Output
	Input() <-chan input.Submission
	Outbound() <-chan UIEvent
	Print(text string)
	Echo(text string)
	SetPrompt(text string)
	SetInput(text string)
	SetInputSubmission(submission input.Submission)

	// Updates
	UpdateBars(content map[string]BarContent)
	UpdateBinds(keys map[string]bool)
	UpdateLayout(top, bottom []LayoutEntry)

	// Components
	ShowPicker(opts ShowPickerMsg)
	SetClipboard(text string)
	CreatePane(name string)
	WritePane(name, text string)
	TogglePane(name string)
	SetPaneVisible(name string, visible bool)
	ClearPane(name string)

	// Input primitives for Lua
	InputSetCursor(pos int)
	OpenEditor(initial string) (string, bool)

	// Pane scrolling primitives for Lua
	PaneScrollUp(name string, lines int)
	PaneScrollDown(name string, lines int)
	PaneScrollToTop(name string)
	PaneScrollToBottom(name string)
}
