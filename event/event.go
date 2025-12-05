package event

// Type identifies the kind of event processed by Session's event loop.
type Type int

const (
	// Data events
	UserInput Type = iota
	NetLine   // A complete line from server (ended with \n)
	NetPrompt // A partial line/prompt (no \n, possibly GA/EOR terminated)

	// Control events
	SysDisconnect // Connection closed

	// Internal
	AsyncResult // Deferred callback execution
)

// Event is a message processed by Session's event loop.
// Payload is nil for events with no associated data.
type Event struct {
	Type    Type
	Payload Payload
}

// Payload is implemented by all event payload types.
// The marker method restricts valid payloads to this package.
type Payload interface {
	eventPayload()
}

// --- Payload Types ---

// Line is the payload for NetLine, NetPrompt, and UserInput events.
type Line string

func (Line) eventPayload() {}

// Callback is the payload for AsyncResult events.
type Callback func()

func (Callback) eventPayload() {}
