package ui

// UI defines the contract for the terminal display layer.
// Implementation lives in the same package (BubbleTeaUI).
type UI interface {
	Run() error
	Quit()
	Done() <-chan struct{}

	// Input/Output
	Input() <-chan string
	Outbound() <-chan UIEvent
	Print(text string)
	Echo(text string)
	SetPrompt(text string)
	SetInput(text string)

	// Updates
	UpdateBars(content map[string]BarContent)
	UpdateBinds(keys map[string]bool)
	UpdateLayout(top, bottom []LayoutEntry)

	// Components
	ShowPicker(title string, items []PickerItem, callbackID string, inline bool)
	CreatePane(name string)
	WritePane(name, text string)
	TogglePane(name string)
	ClearPane(name string)

	// Input primitives for Lua
	InputSetCursor(pos int)
	SetGhost(text string) // Ghost text for command suggestions
	OpenEditor(initial string) (string, bool)

	// Pane scrolling primitives for Lua
	PaneScrollUp(name string, lines int)
	PaneScrollDown(name string, lines int)
	PaneScrollToTop(name string)
	PaneScrollToBottom(name string)
}
