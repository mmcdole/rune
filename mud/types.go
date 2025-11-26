package mud

// EventType identifies the source of the message
type EventType int

const (
	EventUserInput EventType = iota
	EventServerLine   // A complete line from server (ended with \n)
	EventServerPrompt // A partial line/prompt (no \n, possibly GA/EOR terminated)
	EventTimer
	EventSystemControl
)

// Control action constants
const (
	ActionQuit       = "quit"
	ActionConnect    = "connect"
	ActionDisconnect = "disconnect"
	ActionReload     = "reload"
	ActionLoad       = "load"
)

// ControlOp contains control operation details
type ControlOp struct {
	Action     string // Use Action* constants
	Address    string
	ScriptPath string
}

// Event is the universal packet sent to the Orchestrator
type Event struct {
	Type     EventType
	Payload  string     // For User/Server text
	Callback func()     // For Timers (Lua Closures)
	Control  ControlOp  // For SystemControl events
}

// Network defines the TCP/Telnet layer
type Network interface {
	Connect(address string) error
	Disconnect()
	Send(data string)     // Non-blocking write
	Output() <-chan Event // Stream of EventServerLine and EventServerPrompt
}

// UI defines the Terminal layer
type UI interface {
	// Core rendering
	Render(text string)       // Render a complete line (with newline)
	RenderPrompt(text string) // Render a prompt (no newline, overwrites previous prompt)
	Input() <-chan string     // Stream from user
	Run() error
	Done() <-chan struct{} // Signals when UI exits

	// Controller methods (no-op for ConsoleUI)
	SetStatus(text string)
	SetInfobar(text string)
	CreatePane(name string)
	WritePane(name, text string)
	TogglePane(name string)
	ClearPane(name string)
	BindPaneKey(key, name string)
}
