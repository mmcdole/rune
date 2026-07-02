package lua

import (
	"time"

	"github.com/mmcdole/rune/ui"
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

	// Session store: a small Go-owned string store that survives
	// script reloads (but not client exit). Lets Lua keep state
	// across the VM teardown of /reload. Durable state belongs in
	// the disk-backed Store* tier below.
	SessionSet(key, value string)
	SessionGet(key string) (string, bool)
	SessionDelete(key string)

	// Durable store: a Go-owned key→JSON store backed by
	// <config>/store.json; survives client exit. Values are raw JSON
	// (the lua package converts Lua values); writes are write-through
	// and atomic. Reads are served from memory.
	StoreSet(key, rawJSON string) error
	StoreGet(key string) (string, bool)
	StoreDelete(key string) error

	// Logging: Go owns the file handle so an active log survives
	// /reload and is flushed/closed on exit. WHAT gets logged (which
	// lines, stripping, headers) is Lua policy (lua/core/57_log.lua).
	LogStart(path string) (string, error) // opens (append); returns resolved path
	LogStop() bool                        // closes; reports whether a log was open
	LogWrite(text string)                 // appends one line; no-op when inactive
	LogStatus() (string, bool)            // active log path, if any

	// State
	OnConfigChange()
}
