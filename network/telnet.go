package network

import (
	"bytes"
	"strings"
	"sync"
)

// Telnet command constants (RFC 854, 857, 885)
const (
	IAC  byte = 255 // Interpret As Command
	DONT byte = 254
	DO   byte = 253
	WONT byte = 252
	WILL byte = 251
	SB   byte = 250 // Subnegotiation Begin
	GA   byte = 249 // Go Ahead
	EL   byte = 248 // Erase Line
	EC   byte = 247 // Erase Character
	AYT  byte = 246 // Are You There
	AO   byte = 245 // Abort Output
	IP   byte = 244 // Interrupt Process
	BRK  byte = 243 // Break
	DM   byte = 242 // Data Mark
	NOP  byte = 241 // No Operation
	SE   byte = 240 // Subnegotiation End
	EOR  byte = 239 // End of Record (RFC 885)
)

// Telnet option codes
const (
	OptEcho            byte = 1
	OptSuppressGoAhead byte = 3
	OptTerminalType    byte = 24
	OptEOR             byte = 25
	OptNAWS            byte = 31 // Negotiate About Window Size
)

// TelnetBuffer manages the stream state and parses telnet protocol
type TelnetBuffer struct {
	buffer      bytes.Buffer
	mu          sync.Mutex
	seenGAOrEOR bool

	// Negotiation responses to send back
	responses []byte

	// Track what we've agreed to
	willEOR bool
	doEOR   bool

	// Local echo state (true = client should locally echo)
	localEcho bool
}

// NewTelnetBuffer initializes the state machine for processing incoming RFC 854 byte streams.
func NewTelnetBuffer() *TelnetBuffer {
	return &TelnetBuffer{localEcho: true}
}

// ProcessBytes takes raw TCP data, strips Telnet codes, updates state
// Returns any telnet negotiation responses that should be sent back to server
func (tb *TelnetBuffer) ProcessBytes(data []byte) []byte {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.seenGAOrEOR = false
	tb.responses = nil

	i := 0
	for i < len(data) {
		b := data[i]

		if b == IAC {
			if i+1 >= len(data) {
				// Incomplete IAC sequence, save for next packet
				// For now, just skip it
				break
			}

			cmd := data[i+1]

			switch cmd {
			case IAC:
				// Escaped 255 byte
				tb.buffer.WriteByte(IAC)
				i += 2

			case GA, EOR:
				// Prompt terminator signal
				tb.seenGAOrEOR = true
				i += 2

			case WILL:
				if i+2 >= len(data) {
					break
				}
				opt := data[i+2]
				tb.handleWill(opt)
				i += 3

			case WONT:
				if i+2 >= len(data) {
					break
				}
				opt := data[i+2]
				tb.handleWont(opt)
				i += 3

			case DO:
				if i+2 >= len(data) {
					break
				}
				opt := data[i+2]
				tb.handleDo(opt)
				i += 3

			case DONT:
				if i+2 >= len(data) {
					break
				}
				opt := data[i+2]
				tb.handleDont(opt)
				i += 3

			case SB:
				// Subnegotiation - find SE
				end := tb.findSubnegEnd(data[i:])
				if end == -1 {
					// Incomplete, wait for more data
					break
				}
				// Skip the entire subnegotiation for now
				// Future: parse GMCP, MSDP, etc.
				i += end

			case NOP, DM, BRK, IP, AO, AYT, EC, EL:
				// Skip these 2-byte commands
				i += 2

			default:
				// Unknown command, skip 2 bytes
				i += 2
			}
		} else {
			// Normal data byte
			tb.buffer.WriteByte(b)
			i++
		}
	}

	return tb.responses
}

// handleWill processes WILL negotiation
func (tb *TelnetBuffer) handleWill(opt byte) {
	switch opt {
	case OptEOR:
		// Server will send EOR - we want this!
		tb.willEOR = true
		tb.responses = append(tb.responses, IAC, DO, OptEOR)

	case OptSuppressGoAhead:
		// Accept suppress go ahead (standard)
		tb.responses = append(tb.responses, IAC, DO, OptSuppressGoAhead)

	case OptEcho:
		// Server will echo; turn off local echo
		tb.localEcho = false
		tb.responses = append(tb.responses, IAC, DO, OptEcho)

	default:
		// Refuse unknown options
		tb.responses = append(tb.responses, IAC, DONT, opt)
	}
}

// handleWont processes WONT negotiation
func (tb *TelnetBuffer) handleWont(opt byte) {
	switch opt {
	case OptEOR:
		tb.willEOR = false
	case OptEcho:
		// Server won't echo; turn on local echo
		tb.localEcho = true
	}
	// Acknowledge with DONT
	tb.responses = append(tb.responses, IAC, DONT, opt)
}

// handleDo processes DO negotiation
func (tb *TelnetBuffer) handleDo(opt byte) {
	switch opt {
	case OptEOR:
		// Server wants us to send EOR - we can do this
		tb.doEOR = true
		tb.responses = append(tb.responses, IAC, WILL, OptEOR)

	case OptTerminalType:
		// We can report terminal type
		tb.responses = append(tb.responses, IAC, WILL, OptTerminalType)

	case OptNAWS:
		// We can send window size
		tb.responses = append(tb.responses, IAC, WILL, OptNAWS)

	default:
		// Refuse unknown options
		tb.responses = append(tb.responses, IAC, WONT, opt)
	}
}

// handleDont processes DONT negotiation
func (tb *TelnetBuffer) handleDont(opt byte) {
	switch opt {
	case OptEOR:
		tb.doEOR = false
	case OptEcho:
		// Server doesn't want us to echo; leave localEcho unchanged
	}
	// Acknowledge with WONT
	tb.responses = append(tb.responses, IAC, WONT, opt)
}

// findSubnegEnd finds the end of a subnegotiation sequence
// Returns the number of bytes to skip, or -1 if incomplete
func (tb *TelnetBuffer) findSubnegEnd(data []byte) int {
	// Looking for IAC SE
	for i := 2; i < len(data)-1; i++ {
		if data[i] == IAC && data[i+1] == SE {
			return i + 2
		}
	}
	return -1
}

// ExtractLines pulls complete lines out of the buffer
// Lines are delimited by \n, \r\n, or \n\r
func (tb *TelnetBuffer) ExtractLines() []string {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	content := tb.buffer.String()
	if len(content) == 0 {
		return nil
	}

	// Find all line breaks
	var lines []string
	var current strings.Builder
	i := 0

	for i < len(content) {
		ch := content[i]

		if ch == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			i++
			// Skip \r if it follows \n
			if i < len(content) && content[i] == '\r' {
				i++
			}
		} else if ch == '\r' {
			lines = append(lines, current.String())
			current.Reset()
			i++
			// Skip \n if it follows \r
			if i < len(content) && content[i] == '\n' {
				i++
			}
		} else {
			current.WriteByte(ch)
			i++
		}
	}

	// Put remainder back in buffer
	tb.buffer.Reset()
	if current.Len() > 0 {
		tb.buffer.WriteString(current.String())
	}

	return lines
}

// GetPending returns the remaining buffer content (potential prompt)
// If consume is true, the buffer is cleared
func (tb *TelnetBuffer) GetPending(consume bool) string {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	result := tb.buffer.String()
	if consume {
		tb.buffer.Reset()
	}
	return result
}

// HasSignal returns true if GA or EOR was seen in the last ProcessBytes call
func (tb *TelnetBuffer) HasSignal() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.seenGAOrEOR
}

// Clear resets the buffer state
func (tb *TelnetBuffer) Clear() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.buffer.Reset()
	tb.seenGAOrEOR = false
}

// HasEORSupport returns true if EOR negotiation succeeded
func (tb *TelnetBuffer) HasEORSupport() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.willEOR || tb.doEOR
}

// LocalEchoEnabled reports whether the client should locally echo input.
func (tb *TelnetBuffer) LocalEchoEnabled() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.localEcho
}
