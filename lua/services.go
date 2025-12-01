package lua

import "time"

// PickerItem represents an item for the picker API.
// Session converts this to ui.GenericItem (decoupling lua from ui).
type PickerItem struct {
	Text        string // Display text
	Description string // Optional description
	Value       string // Value returned on selection (defaults to Text if empty)
	MatchDesc   bool   // If true, include Description in fuzzy matching
}

// NetworkService handles connection logic.
type NetworkService interface {
	Connect(addr string)
	Disconnect()
	Send(data string)
}

// UIService handles visual elements.
type UIService interface {
	Print(text string)
	SetStatus(text string)
	SetInfobar(text string)

	// Pane operations
	PaneCreate(name string)
	PaneWrite(name, text string)
	PaneToggle(name string)
	PaneClear(name string)

	// Picker
	ShowPicker(title string, items []PickerItem, onSelect func(value string), inline bool)

	// Input
	GetInput() string
	SetInput(text string)
}

// TimerService handles scheduling.
type TimerService interface {
	TimerAfter(d time.Duration) int
	TimerEvery(d time.Duration) int
	TimerCancel(id int)
	TimerCancelAll()
}

// SystemService handles app lifecycle.
type SystemService interface {
	Quit()
	Reload()
	Load(path string)
}

// HistoryService handles input history.
type HistoryService interface {
	GetHistory() []string
	AddToHistory(cmd string)
}

// StateService provides read-only access to client state.
type StateService interface {
	GetClientState() ClientState
	OnConfigChange()
}
