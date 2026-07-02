package network

// OutputKind identifies the type of network output.
type OutputKind int

const (
	OutputLine        OutputKind = iota // Complete line from server
	OutputPrompt                        // Partial line/prompt (GA/EOR terminated or unterminated)
	OutputDisconnect                    // Connection closed
	OutputGMCP                          // GMCP message (Package + raw JSON Payload)
	OutputGMCPEnabled                   // GMCP negotiation completed for this connection
)

// Output represents data emitted by the network layer.
type Output struct {
	Kind    OutputKind
	Payload string // Line content, or raw JSON for GMCP (may be empty)
	Package string // GMCP package name (e.g. "Char.Vitals"); GMCP only
}
