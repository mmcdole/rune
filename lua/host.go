package lua

import "time"

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
	PaneOp(op, name, data string)

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
