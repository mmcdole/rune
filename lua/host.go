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

	// UI
	SetStatus(text string)
	SetInfobar(text string)
	Pane(op, name, data string)

	// Timer scheduling - Session owns Go timers, Engine owns Lua callbacks
	ScheduleTimer(id int, d time.Duration)
	CancelTimer(id int)
	CancelAllTimers()
}
