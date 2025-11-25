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

// ScriptEngine describes the Lua interactions
type ScriptEngine interface {
	// Init loads the Lua state and scripts
	Init(corePath, userPath string) error

	// RegisterHostFuncs allows Go to bind 'client.send', 'client.timer'
	// The 'out' channel is how the Engine sends commands back to the Orchestrator
	RegisterHostFuncs(out chan<- Event, sendToNet chan<- string, renderToUI chan<- string)

	// OnInput handles user typing. Returns true if handled.
	OnInput(text string) bool

	// OnOutput handles server text. Returns modified text and boolean (false = gag).
	OnOutput(text string) (string, bool)

	// OnPrompt handles server prompts. Returns modified text.
	OnPrompt(text string) string

	// ExecuteCallback runs a stored Lua function (from a timer)
	ExecuteCallback(cb func())

	// Close cleans up
	Close()
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
	Render(text string)       // Render a complete line (with newline)
	RenderPrompt(text string) // Render a prompt (no newline, overwrites previous prompt)
	Input() <-chan string     // Stream from user
	Run() error
}

// UIControlType identifies the type of UI control message
type UIControlType int

const (
	UIControlStatus UIControlType = iota
	UIControlPaneCreate
	UIControlPaneWrite
	UIControlPaneToggle
	UIControlPaneClear
	UIControlPaneBind
	UIControlInfobar
)

// UIControl is a message for controlling the TUI from Lua
type UIControl struct {
	Type UIControlType
	Text string // For status, pane write
	Name string // Pane name
	Key  string // For key bindings
}
