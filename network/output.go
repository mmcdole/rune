package network

// OutputKind identifies the type of network output.
type OutputKind int

const (
	OutputLine    OutputKind = iota // Complete line from the server
	OutputPartial                   // Current partial line
	OutputPrompt                    // Prompt terminated by Telnet GA/EOR
	// OutputSendBoundary marks one game-line send after dropping the partial line.
	OutputSendBoundary
	OutputDisconnect  // Connection closed
	OutputGMCP        // GMCP message (Package + raw JSON Payload)
	OutputGMCPEnabled // GMCP negotiation completed for this connection
)

// Output represents data emitted by the network layer.
type Output struct {
	Kind         OutputKind
	ConnectionID uint64 // Connection that produced this output
	Payload      string // Line content, or raw JSON for GMCP (may be empty)
	Package      string // GMCP package name (e.g. "Char.Vitals"); GMCP only
}
