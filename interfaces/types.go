package interfaces

// BarContent holds the rendered content of a bar.
type BarContent struct {
	Left   string
	Center string
	Right  string
}

// PickerItem represents an item for picker/selection UI.
type PickerItem struct {
	Text        string
	Description string
	Value       string // ID or Value passed back to caller
	MatchDesc   bool   // If true, include Description in fuzzy matching
}

// EventType identifies the source of the message
type EventType int

const (
	EventUserInput EventType = iota
	EventNetLine             // A complete line from server (ended with \n)
	EventNetPrompt           // A partial line/prompt (no \n, possibly GA/EOR terminated)
	EventTimer
	EventSystemControl
	EventAsyncResult // Async work completion dispatched onto the session loop
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

// UI defines the Terminal layer.
// Single implementation: BubbleTeaUI.
type UI interface {
	// --- Lifecycle ---
	Run() error
	Quit()
	Done() <-chan struct{}
	Input() <-chan string
	Outbound() <-chan any

	// --- Main Output Stream ---
	// Appends text to the main scrollback buffer.
	// Implementation handles batching/performance (16ms tick).
	Print(text string)

	// --- Semantic Output ---
	// Appends user input to scrollback with local-echo styling.
	Echo(text string)

	// Updates the active server prompt (overlay at bottom).
	SetPrompt(text string)

	// --- Reactive State (Push Architecture) ---
	UpdateBars(content map[string]BarContent)
	UpdateBinds(keys map[string]bool)
	UpdateLayout(top, bottom []string)
	SetInput(text string)

	// --- Interactive Elements ---
	ShowPicker(title string, items []PickerItem, callbackID string, inline bool)

	// --- Pane Management ---
	CreatePane(name string)
	WritePane(name, text string)
	TogglePane(name string)
	ClearPane(name string)
}
