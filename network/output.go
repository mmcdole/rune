package network

// OutputKind identifies the type of network output.
type OutputKind int

const (
	OutputLine       OutputKind = iota // Complete line from server
	OutputPrompt                       // Partial line/prompt (GA/EOR terminated or unterminated)
	OutputDisconnect                   // Connection closed
)

// Output represents data emitted by the network layer.
type Output struct {
	Kind    OutputKind
	Payload string // Line content (empty for Disconnect)
}
