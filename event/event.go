package event

// Type identifies the kind of event sent to the Orchestrator
type Type int

const (
	// Data events
	UserInput Type = iota
	NetLine   // A complete line from server (ended with \n)
	NetPrompt // A partial line/prompt (no \n, possibly GA/EOR terminated)

	// Control events (promoted to top level for type safety)
	SysQuit
	SysConnect    // Payload = "address:port"
	SysDisconnect
	SysReload
	SysLoadScript // Payload = "path/to/script.lua"

	// Internal
	Timer
	AsyncResult // Async work completion dispatched onto the session loop
)

// Event is the universal packet sent to the Orchestrator
type Event struct {
	Type     Type
	Payload  string // For User/Server text, or control event data
	Callback func() // For Timers (Lua Closures)
}
