package mud

// EventType identifies the source of the message
type EventType int

const (
	EventUserInput EventType = iota
	EventNetLine             // A complete line from server (ended with \n)
	EventNetPrompt           // A partial line/prompt (no \n, possibly GA/EOR terminated)
	EventTimer
	EventSystemControl
	EventAsyncResult   // Async work completion dispatched onto the session loop
	EventDisplayLine   // Append a line to scrollback (server output or prompt commit)
	EventDisplayEcho   // Append a local echo line (e.g., "> cmd")
	EventDisplayPrompt // Set/update the live prompt overlay
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
	Type     EventType
	Payload  string    // For User/Server text
	Callback func()    // For Timers (Lua Closures)
	Control  ControlOp // For SystemControl events
}

// Network defines the TCP/Telnet layer
type Network interface {
	Connect(address string) error
	Disconnect()
	Send(data string)     // Non-blocking write
	Output() <-chan Event // Stream of EventNetLine and EventNetPrompt
}

// UI defines the Terminal layer
type UI interface {
	// Core rendering
	Render(text string)            // Render a complete line (legacy helper)
	RenderDisplayLine(text string) // Render a line into scrollback (server output or prompt commit)
	RenderEcho(text string)        // Render a user echo line (e.g., "> cmd")
	RenderPrompt(text string)      // Render a prompt overlay (no newline, overwrites previous prompt)
	Input() <-chan string          // Stream from user
	Run() error
	Done() <-chan struct{} // Signals when UI exits
	Quit()                 // Request UI to exit

	// Controller methods (no-op for ConsoleUI)
	SetStatus(text string)
	SetInfobar(text string)
	CreatePane(name string)
	WritePane(name, text string)
	TogglePane(name string)
	ClearPane(name string)
	BindPaneKey(key, name string)
}
