package lua

import (
	"time"

	"github.com/drake/rune/ui"
)

// Host provides all services the Lua engine needs from the host application.
// In production, Session implements this interface.
type Host interface {
	// Network
	Send(data string) error
	Connect(addr string)
	Disconnect()

	// UI
	Print(text string)
	PaneCreate(name string)
	PaneWrite(name, text string)
	PaneToggle(name string)
	PaneClear(name string)
	ShowPicker(title string, items []ui.PickerItem, callbackID string, inline bool)
	GetInput() string
	SetInput(text string)

	// Timers
	TimerAfter(d time.Duration) int
	TimerEvery(d time.Duration) int
	TimerCancel(id int)
	TimerCancelAll()

	// System
	Quit()
	Reload()
	Load(path string)

	// History
	GetHistory() []string
	AddToHistory(cmd string)

	// State
	GetClientState() ClientState
	OnConfigChange()
}
