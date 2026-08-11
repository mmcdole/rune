package network

// OutputKind identifies the type of network output.
type OutputKind int

const (
	OutputLine    OutputKind = iota // Delimiter-terminated line from server
	OutputPartial                   // Current unfinished text tail (not consumed)
	OutputPrompt                    // Text tail terminated by Telnet GA/EOR
	// OutputSendBoundary is an internal control event emitted before every
	// outbound command line. It marks where the unfinished accumulator was
	// discarded; later server text is ordered behind it.
	OutputSendBoundary
	OutputDisconnect  // Connection closed
	OutputGMCP        // GMCP message (Package + raw JSON Payload)
	OutputGMCPEnabled // GMCP negotiation completed for this connection
)

// PromptTerminator identifies the Telnet command that confirmed a prompt.
// It is meaningful only for OutputPrompt.
type PromptTerminator int

const (
	PromptTerminatorNone PromptTerminator = iota
	PromptTerminatorGA
	PromptTerminatorEOR
)

// Output represents data emitted by the network layer.
type Output struct {
	Kind             OutputKind
	Payload          string           // Line content, or raw JSON for GMCP (may be empty)
	Package          string           // GMCP package name (e.g. "Char.Vitals"); GMCP only
	PromptTerminator PromptTerminator // OutputPrompt only
}
