package ui

// UI defines the contract for the terminal display layer.
// Implementation lives in the same package (BubbleTeaUI).
type UI interface {
	Run() error
	Quit()
	Done() <-chan struct{}

	// Input/Output
	Input() <-chan string
	Outbound() <-chan any
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
}
