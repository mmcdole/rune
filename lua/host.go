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
	Pane(op, name, data string)

	// Timers - Timer service owns IDs, scheduling, and cancellation
	TimerAfter(d time.Duration) int
	TimerEvery(d time.Duration) int
	TimerCancel(id int)
	TimerCancelAll()
}
