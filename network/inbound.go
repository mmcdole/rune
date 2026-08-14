package network

// InboundKind identifies a message produced by a transport connection.
type InboundKind uint8

const (
	// InboundBatch carries the Session-facing events from one parser call. The
	// event slice keeps the order in which bytes appeared on the wire.
	InboundBatch InboundKind = iota
	// InboundDisconnect reports that the connection can no longer be read.
	InboundDisconnect
)

// EventBatch owns the Session-facing events from one Parser.Receive call after
// the transport has removed MCCP activation events. Secure describes the
// connection that supplied those bytes.
type EventBatch struct {
	Secure bool
	Events []TelnetEvent
}

// Inbound is one transport message to Session: an event batch or a
// disconnect, tagged with the connection that produced it.
type Inbound struct {
	Kind         InboundKind
	ConnectionID uint64
	Batch        EventBatch
}
