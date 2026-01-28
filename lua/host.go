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

	// Input primitives
	InputGetCursor() int
	InputSetCursor(pos int)
	SetGhost(text string) // Ghost text for command suggestions
	OpenEditor(initial string) (string, bool)

	// Pane scrolling
	PaneScrollUp(name string, lines int)
	PaneScrollDown(name string, lines int)
	PaneScrollToTop(name string)
	PaneScrollToBottom(name string)

	// Timers
	TimerAfter(d time.Duration) int
	TimerEvery(d time.Duration) int
	TimerCancel(id int)
	TimerCancelAll()

	// System
	Quit()
	Reload()
	Load(path string)
	RefreshBars() // Force immediate bar refresh

	// History
	GetHistory() []string
	AddToHistory(cmd string)

	// State
	OnConfigChange()
}
