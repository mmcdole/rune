package event

// Type identifies the source of the message
type Type int

const (
	UserInput Type = iota
	NetLine        // A complete line from server (ended with \n)
	NetPrompt      // A partial line/prompt (no \n, possibly GA/EOR terminated)
	Timer
	SystemControl
	AsyncResult // Async work completion dispatched onto the session loop
)

// Control action constants
const (
	ActionQuit       = "quit"
	ActionConnect    = "connect"
	ActionDisconnect = "disconnect"
	ActionReload     = "reload"
	ActionLoadScript = "load_script"
)

// ControlOp contains control operation details
type ControlOp struct {
	Action     string // Use Action* constants
	Address    string
	ScriptPath string
}

// Event is the universal packet sent to the Orchestrator
type Event struct {
	Type     Type
	Payload  string    // For User/Server text
	Callback func()    // For Timers (Lua Closures)
	Control  ControlOp // For SystemControl events
}
