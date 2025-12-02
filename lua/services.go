package lua

import (
	"time"

	"github.com/drake/rune/ui"
)

// NetworkService handles connection logic.
type NetworkService interface {
	Connect(addr string)
	Disconnect()
	Send(data string)
}

// UIService handles visual elements.
type UIService interface {
	Print(text string)

	// Pane operations
	PaneCreate(name string)
	PaneWrite(name, text string)
	PaneToggle(name string)
	PaneClear(name string)

	// Picker
	ShowPicker(title string, items []ui.PickerItem, onSelect func(value string), inline bool)

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
