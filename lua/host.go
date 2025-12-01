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

// Host provides the bridge between Engine and the rest of the system.
// This abstraction decouples Engine from specific implementations,
// making it testable without full infrastructure.
type Host interface {
	// IO
	Print(text string)
	Send(data string)

	// System / Lifecycle
	Quit()
	Connect(addr string)
	Disconnect()
	Reload()
	Load(path string)

	// UI
	SetStatus(text string)
	SetInfobar(text string)

	// Pane operations
	PaneCreate(name string)
	PaneWrite(name, text string)
	PaneToggle(name string)
	PaneClear(name string)

	// Picker - Generic picker overlay for Lua-driven selection UI
	// inline: if true, picker filters based on input; if false, picker captures keyboard
	ShowPicker(title string, items []PickerItem, onSelect func(value string), inline bool)

	// History - Get/Add input history
	GetHistory() []string
	AddToHistory(cmd string)

	// Input - Set input line content (for picker selection)
	SetInput(text string)

	// Timers - Timer service owns IDs, scheduling, and cancellation
	TimerAfter(d time.Duration) int
	TimerEvery(d time.Duration) int
	TimerCancel(id int)
	TimerCancelAll()

	// State - Get current client state for Lua
	GetClientState() ClientState

	// OnConfigChange notifies the host that binds or layout have changed.
	// Called synchronously from Lua when rune.bind, rune.unbind, or
	// rune.ui.layout is called, allowing the host to push updates to the UI.
	OnConfigChange()
}
