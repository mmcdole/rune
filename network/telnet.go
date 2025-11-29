// Package network provides telnet protocol parsing for MUD clients.
// This is a faithful Go port of libmudtelnet (Rust).
package network

import (
	"bytes"
)

// Telnet command codes (op_command in libmudtelnet).
const (
	CmdIAC  byte = 255 // Interpret As Command
	CmdWILL byte = 251 // Will use option
	CmdWONT byte = 252 // Won't use option
	CmdDO   byte = 253 // Do use option
	CmdDONT byte = 254 // Don't use option
	CmdNOP  byte = 241 // No operation
	CmdSB   byte = 250 // Subnegotiation begin
	CmdSE   byte = 240 // Subnegotiation end
	CmdIS   byte = 0   // Subnegotiation IS
	CmdSEND byte = 1   // Subnegotiation SEND
	CmdGA   byte = 249 // Go ahead
	CmdEOR  byte = 239 // End of record
)

// Telnet option codes (op_option in libmudtelnet).
const (
	OptBinary         byte = 0
	OptEcho           byte = 1
	OptRCP            byte = 2
	OptSGA            byte = 3 // Suppress Go Ahead
	OptNAMS           byte = 4
	OptStatus         byte = 5
	OptTM             byte = 6
	OptRCTE           byte = 7
	OptNAOL           byte = 8
	OptNAOP           byte = 9
	OptNAOCRD         byte = 10
	OptNAOHTS         byte = 11
	OptNAOHTD         byte = 12
	OptNAOFFD         byte = 13
	OptNAOVTS         byte = 14
	OptNAOVTD         byte = 15
	OptNAOLFD         byte = 16
	OptXASCII         byte = 17
	OptLogout         byte = 18
	OptBM             byte = 19
	OptDET            byte = 20
	OptSUPDUP         byte = 21
	OptSUPDUPOutput   byte = 22
	OptSNDLOC         byte = 23
	OptTTYPE          byte = 24
	OptEOR            byte = 25
	OptTUID           byte = 26
	OptOUTMRK         byte = 27
	OptTTYLOC         byte = 28
	Opt3270Regime     byte = 29
	OptX3PAD          byte = 30
	OptNAWS           byte = 31
	OptTSPEED         byte = 32
	OptLFLOW          byte = 33
	OptLinemode       byte = 34
	OptXDISPLOC       byte = 35
	OptEnviron        byte = 36
	OptAuthentication byte = 37
	OptEncrypt        byte = 38
	OptNewEnviron     byte = 39
	OptMSSP           byte = 70
	OptMCCP2          byte = 86
	OptMCCP3          byte = 87
	OptZMP            byte = 93
	OptEXOPL          byte = 255
	OptGMCP           byte = 201
)

// Aliases for backwards compatibility
const (
	OptSuppressGA = OptSGA
	OptLINEMODE   = OptLinemode
)

// TelnetEventKind represents the type of telnet event.
type TelnetEventKind int

const (
	TelnetEventDataReceive TelnetEventKind = iota
	TelnetEventDataSend
	TelnetEventIAC
	TelnetEventNegotiation
	TelnetEventSubnegotiation
	TelnetEventDecompressImmediate
)

// TelnetEvent carries parser output.
type TelnetEvent struct {
	Kind    TelnetEventKind
	Command byte   // For IAC, Negotiation
	Option  byte   // For Negotiation, Subnegotiation
	Data    []byte // For DataReceive, DataSend, Subnegotiation, DecompressImmediate
}

// --- Compatibility table (port of libmudtelnet/src/compatibility.rs) ---

// Bitmask constants for CompatibilityTable
const (
	bitLocal       byte = 1      // 0x01 - Option is locally supported
	bitRemote      byte = 1 << 1 // 0x02 - Option is remotely supported
	bitLocalState  byte = 1 << 2 // 0x04 - Option is currently enabled locally
	bitRemoteState byte = 1 << 3 // 0x08 - Option is currently enabled remotely
)

// CompatibilityEntry represents the negotiation state for a single telnet option.
type CompatibilityEntry struct {
	Local       bool // We support this option locally (us -> them)
	Remote      bool // We support this option remotely (them -> us)
	LocalState  bool // Option currently enabled locally
	RemoteState bool // Option currently enabled remotely
}

// toU8 converts a CompatibilityEntry to a bitmask.
func (e CompatibilityEntry) toU8() byte {
	var res byte
	if e.Local {
		res |= bitLocal
	}
	if e.Remote {
		res |= bitRemote
	}
	if e.LocalState {
		res |= bitLocalState
	}
	if e.RemoteState {
		res |= bitRemoteState
	}
	return res
}

// entryFromU8 creates a CompatibilityEntry from a bitmask.
func entryFromU8(value byte) CompatibilityEntry {
	return CompatibilityEntry{
		Local:       value&bitLocal == bitLocal,
		Remote:      value&bitRemote == bitRemote,
		LocalState:  value&bitLocalState == bitLocalState,
		RemoteState: value&bitRemoteState == bitRemoteState,
	}
}

// CompatibilityTable tracks negotiation state for all 256 telnet options.
// Uses compact bitmask representation (4 bits per option).
type CompatibilityTable struct {
	options [256]byte
}

// NewCompatibilityTable creates a new empty compatibility table.
func NewCompatibilityTable() CompatibilityTable {
	return CompatibilityTable{}
}

// DefaultCompatibility returns the default compatibility table for MUD clients.
// This enables support for common MUD-related telnet options.
func DefaultCompatibility() CompatibilityTable {
	return defaultCompatibility()
}

// FromOptions creates a table with specific option values set.
// Each tuple is (option, bitmask).
func FromOptions(values [][2]byte) CompatibilityTable {
	table := CompatibilityTable{}
	for _, v := range values {
		table.options[v[0]] = v[1]
	}
	return table
}

// SupportLocal enables local support for an option.
func (t *CompatibilityTable) SupportLocal(option byte) {
	entry := t.Get(option)
	entry.Local = true
	t.Set(option, entry)
}

// SupportRemote enables remote support for an option.
func (t *CompatibilityTable) SupportRemote(option byte) {
	entry := t.Get(option)
	entry.Remote = true
	t.Set(option, entry)
}

// Support enables both local and remote support for an option.
func (t *CompatibilityTable) Support(option byte) {
	entry := t.Get(option)
	entry.Local = true
	entry.Remote = true
	t.Set(option, entry)
}

// Get retrieves the current state for an option.
func (t *CompatibilityTable) Get(option byte) CompatibilityEntry {
	return entryFromU8(t.options[option])
}

// Set updates the state for an option.
func (t *CompatibilityTable) Set(option byte, entry CompatibilityEntry) {
	t.options[option] = entry.toU8()
}

// ResetStates resets all negotiated states while preserving support flags.
func (t *CompatibilityTable) ResetStates() {
	for i := range t.options {
		entry := entryFromU8(t.options[i])
		entry.LocalState = false
		entry.RemoteState = false
		t.options[i] = entry.toU8()
	}
}

// --- Parser (port of libmudtelnet/src/lib.rs) ---

// Parser is a telnet protocol parser.
type Parser struct {
	Options CompatibilityTable
	buffer  []byte
}

// NewParser creates a parser with the given compatibility table.
func NewParser(table CompatibilityTable) *Parser {
	return &Parser{
		Options: table,
		buffer:  make([]byte, 0, 128),
	}
}

// NewParserDefault creates a parser with default (empty) compatibility.
func NewParserDefault() *Parser {
	return NewParser(NewCompatibilityTable())
}

// NewParserWithCapacity creates a parser with a specified initial buffer capacity.
func NewParserWithCapacity(size int) *Parser {
	return &Parser{
		buffer: make([]byte, 0, size),
	}
}

// Receive ingests data and returns parsed events.
func (p *Parser) Receive(data []byte) []TelnetEvent {
	p.buffer = append(p.buffer, data...)
	return p.process()
}

// LinemodeEnabled reports whether remote linemode is enabled.
func (p *Parser) LinemodeEnabled() bool {
	entry := p.Options.Get(OptLinemode)
	return entry.Remote && entry.RemoteState
}

// EscapeIAC doubles IAC bytes for outbound data.
// Example: [255, 1, 6, 2] -> [255, 255, 1, 6, 2]
func EscapeIAC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		out = append(out, b)
		if b == CmdIAC {
			out = append(out, CmdIAC)
		}
	}
	return out
}

// UnescapeIAC collapses doubled IAC bytes in received data.
// Example: [255, 255, 1, 6, 2] -> [255, 1, 6, 2]
// This matches libmudtelnet's fixed unescape logic.
func UnescapeIAC(data []byte) []byte {
	const (
		stNormal = iota
		stIAC
	)

	out := make([]byte, 0, len(data))
	state := stNormal

	for _, val := range data {
		switch state {
		case stNormal:
			if val == CmdIAC {
				state = stIAC
				out = append(out, val)
			} else {
				out = append(out, val)
			}
		case stIAC:
			if val == CmdIAC {
				// Doubled IAC -> consume one, output nothing extra
				state = stNormal
			} else {
				out = append(out, val)
				state = stNormal
			}
		}
	}

	return out
}

// Negotiate creates a negotiation event (IAC + command + option).
func (p *Parser) Negotiate(command, option byte) TelnetEvent {
	return TelnetEvent{
		Kind: TelnetEventDataSend,
		Data: []byte{CmdIAC, command, option},
	}
}

// Will indicates we want to use an option locally.
// Returns nil if option is not locally supported or already enabled.
func (p *Parser) Will(option byte) *TelnetEvent {
	entry := p.Options.Get(option)
	if entry.Local && !entry.LocalState {
		entry.LocalState = true
		p.Options.Set(option, entry)
		ev := p.Negotiate(CmdWILL, option)
		return &ev
	}
	return nil
}

// Wont indicates we don't want to use an option locally.
// Returns nil if option is already disabled.
func (p *Parser) Wont(option byte) *TelnetEvent {
	entry := p.Options.Get(option)
	if entry.LocalState {
		entry.LocalState = false
		p.Options.Set(option, entry)
		ev := p.Negotiate(CmdWONT, option)
		return &ev
	}
	return nil
}

// Do requests the remote end to use an option.
// Returns nil if option is not remotely supported or already enabled.
func (p *Parser) Do(option byte) *TelnetEvent {
	entry := p.Options.Get(option)
	if entry.Remote && !entry.RemoteState {
		ev := p.Negotiate(CmdDO, option)
		return &ev
	}
	return nil
}

// Dont requests the remote end to stop using an option.
// Returns nil if option is already disabled.
func (p *Parser) Dont(option byte) *TelnetEvent {
	entry := p.Options.Get(option)
	if entry.RemoteState {
		ev := p.Negotiate(CmdDONT, option)
		return &ev
	}
	return nil
}

// Subnegotiation sends a subnegotiation for a locally supported and enabled option.
// Returns nil if option is not locally supported or not enabled.
func (p *Parser) Subnegotiation(option byte, data []byte) *TelnetEvent {
	entry := p.Options.Get(option)
	if entry.Local && entry.LocalState {
		escaped := EscapeIAC(data)
		buf := make([]byte, 0, 3+len(escaped)+2)
		buf = append(buf, CmdIAC, CmdSB, option)
		buf = append(buf, escaped...)
		buf = append(buf, CmdIAC, CmdSE)
		return &TelnetEvent{
			Kind:   TelnetEventDataSend,
			Option: option,
			Data:   buf,
		}
	}
	return nil
}

// SubnegotiationText sends a subnegotiation with text data.
func (p *Parser) SubnegotiationText(option byte, text string) *TelnetEvent {
	return p.Subnegotiation(option, []byte(text))
}

// SendText prepares text for transmission with CRLF and IAC escaping.
func SendText(text string) TelnetEvent {
	escaped := EscapeIAC([]byte(text + "\r\n"))
	return TelnetEvent{Kind: TelnetEventDataSend, Data: escaped}
}

// --- Internal parsing helpers ---

type eventType int

const (
	evNone eventType = iota
	evIAC
	evNeg
	evSub
)

type parsedSlice struct {
	kind      eventType
	buf       []byte
	remaining []byte // only for subneg when MCCP decompression has a tail
}

// process runs extract -> decode, matching libmudtelnet's process().
func (p *Parser) process() []TelnetEvent {
	var out []TelnetEvent
	events := p.extract()
	for _, ev := range events {
		switch ev.kind {
		case evNone, evIAC, evNeg:
			out = append(out, p.processCommand(ev.buf)...)
		case evSub:
			out = append(out, p.processSub(ev.buf, ev.remaining)...)
		}
	}
	return out
}

// extract mirrors libmudtelnet::extract_event_data with state machine.
func (p *Parser) extract() []parsedSlice {
	const (
		stateNormal = iota
		stateIAC
		stateNeg
		stateSub
		stateSubOpt
		stateSubIAC
	)

	var res []parsedSlice
	state := stateNormal
	cmdBegin := 0
	var subOpt byte
	buf := p.buffer
	p.buffer = nil // Take ownership of buffer

	for i := 0; i < len(buf); i++ {
		val := buf[i]
		switch state {
		case stateNormal:
			if val == CmdIAC {
				if cmdBegin != i {
					res = append(res, parsedSlice{kind: evNone, buf: buf[cmdBegin:i]})
				}
				state = stateIAC
				cmdBegin = i
			}

		case stateIAC:
			switch val {
			case CmdIAC:
				// Double IAC - treat as escaped data, stay in normal state
				state = stateNormal
			case CmdGA, CmdEOR, CmdNOP:
				res = append(res, parsedSlice{kind: evIAC, buf: buf[cmdBegin : i+1]})
				state = stateNormal
				cmdBegin = i + 1
			case CmdSB:
				state = stateSub
			default: // WILL, WONT, DO, DONT, etc.
				state = stateNeg
			}

		case stateNeg:
			// Complete negotiation: IAC + command + option
			res = append(res, parsedSlice{kind: evNeg, buf: buf[cmdBegin : i+1]})
			state = stateNormal
			cmdBegin = i + 1

		case stateSub:
			// Capture the option byte
			subOpt = val
			state = stateSubOpt

		case stateSubOpt:
			if val == CmdIAC {
				state = stateSubIAC
			}

		case stateSubIAC:
			if val == CmdSE {
				// Check for MCCP2/3: remaining data after SE must be decompressed
				if (subOpt == OptMCCP2 || subOpt == OptMCCP3) && i+1 < len(buf) {
					res = append(res, parsedSlice{
						kind:      evSub,
						buf:       buf[cmdBegin : i+1],
						remaining: buf[i+1:],
					})
					cmdBegin = len(buf)
					i = len(buf) - 1 // Will be incremented to len(buf)
				} else {
					res = append(res, parsedSlice{kind: evSub, buf: buf[cmdBegin : i+1]})
					state = stateNormal
					cmdBegin = i + 1
				}
			} else if val == CmdIAC {
				// Escaped IAC within subnegotiation - stay in stateSubIAC
			} else {
				// Non-SE after IAC in subneg - back to collecting data
				state = stateSubOpt
			}
		}
	}

	// Handle remaining data at end of buffer
	if cmdBegin < len(buf) {
		switch state {
		case stateSub, stateSubOpt, stateSubIAC:
			// Incomplete subnegotiation - put back in buffer
			p.buffer = append(p.buffer, buf[cmdBegin:]...)
		case stateIAC, stateNeg:
			// Incomplete IAC or negotiation sequence - put back in buffer
			p.buffer = append(p.buffer, buf[cmdBegin:]...)
		default:
			// Regular data
			res = append(res, parsedSlice{kind: evNone, buf: buf[cmdBegin:]})
		}
	}

	return res
}

func (p *Parser) processCommand(buf []byte) []TelnetEvent {
	var out []TelnetEvent

	// Check for IAC sequences
	if len(buf) >= 2 && buf[0] == CmdIAC {
		cmd := buf[1]
		if cmd != CmdSE { // Ignore stray SE
			if len(buf) == 2 {
				// Simple IAC command (GA, EOR, NOP, etc.)
				out = append(out, TelnetEvent{Kind: TelnetEventIAC, Command: cmd})
			} else if len(buf) == 3 {
				// Negotiation command
				out = append(out, p.processNegotiation(buf[1], buf[2])...)
			}
		}
	} else if len(buf) > 0 && buf[0] != CmdIAC {
		// Plain data
		out = append(out, TelnetEvent{Kind: TelnetEventDataReceive, Data: buf})
	}

	return out
}

func (p *Parser) processSub(buf, remaining []byte) []TelnetEvent {
	// Check for valid ending: must end with IAC SE
	if len(buf) < 5 || buf[len(buf)-2] != CmdIAC || buf[len(buf)-1] != CmdSE {
		// Incomplete subnegotiation - put back in buffer
		p.buffer = append(p.buffer, buf...)
		return nil
	}

	opt := buf[2]
	entry := p.Options.Get(opt)
	if !entry.Local || !entry.LocalState {
		// Ignore subnegotiations for unsupported/disabled options
		return nil
	}

	// Extract and unescape payload (between option and IAC SE)
	payload := UnescapeIAC(buf[3 : len(buf)-2])

	events := []TelnetEvent{{
		Kind:   TelnetEventSubnegotiation,
		Option: opt,
		Data:   payload,
	}}

	// For MCCP2/3, emit DecompressImmediate with remaining data
	if (opt == OptMCCP2 || opt == OptMCCP3) && len(remaining) > 0 {
		events = append(events, TelnetEvent{
			Kind: TelnetEventDecompressImmediate,
			Data: remaining,
		})
	}

	return events
}

// processNegotiation handles WILL/WONT/DO/DONT sequences.
// This matches libmudtelnet's process_negotiation logic.
func (p *Parser) processNegotiation(command, opt byte) []TelnetEvent {
	entry := p.Options.Get(opt)
	var responses []TelnetEvent

	switch command {
	case CmdWILL:
		if entry.Remote && !entry.RemoteState {
			// Accept: enable remote state and send DO
			entry.RemoteState = true
			p.Options.Set(opt, entry)
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventDataSend,
				Data: []byte{CmdIAC, CmdDO, opt},
			})
			responses = append(responses, TelnetEvent{
				Kind:    TelnetEventNegotiation,
				Command: command,
				Option:  opt,
			})
		} else if !entry.Remote {
			// Reject: send DONT (no negotiation event)
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventDataSend,
				Data: []byte{CmdIAC, CmdDONT, opt},
			})
		}

	case CmdWONT:
		if entry.RemoteState {
			// Disable remote state
			entry.RemoteState = false
			p.Options.Set(opt, entry)
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventDataSend,
				Data: []byte{CmdIAC, CmdDONT, opt},
			})
		}
		responses = append(responses, TelnetEvent{
			Kind:    TelnetEventNegotiation,
			Command: command,
			Option:  opt,
		})

	case CmdDO:
		if entry.Local && !entry.LocalState {
			// Accept: enable local and remote state, send WILL
			entry.LocalState = true
			entry.RemoteState = true
			p.Options.Set(opt, entry)
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventDataSend,
				Data: []byte{CmdIAC, CmdWILL, opt},
			})
			responses = append(responses, TelnetEvent{
				Kind:    TelnetEventNegotiation,
				Command: command,
				Option:  opt,
			})
		} else if !entry.Local {
			// Not locally supported: reject with WONT
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventDataSend,
				Data: []byte{CmdIAC, CmdWONT, opt},
			})
		}
		// If already enabled (entry.LocalState == true), do nothing

	case CmdDONT:
		if entry.LocalState {
			// Disable local state
			entry.LocalState = false
			p.Options.Set(opt, entry)
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventDataSend,
				Data: []byte{CmdIAC, CmdWONT, opt},
			})
		}
		responses = append(responses, TelnetEvent{
			Kind:    TelnetEventNegotiation,
			Command: command,
			Option:  opt,
		})
	}

	return responses
}

// --- OutputBuffer (line/prompt splitting, not part of libmudtelnet) ---

// TelnetMode represents how prompt/line boundaries are detected.
type TelnetMode int

const (
	TelnetModeUnterminated     TelnetMode = iota // Split on \n, prompts are unterminated
	TelnetModeTerminatedPrompt                   // Prompts terminated by GA/EOR
)

// OutputBuffer buffers incoming data and splits it into lines.
type OutputBuffer struct {
	buffer  bytes.Buffer
	mode    TelnetMode
	newData bool // Tracks if buffer has new data since last prompt flush
}

// NewOutputBuffer creates a new output buffer with the specified mode.
func NewOutputBuffer(mode TelnetMode) *OutputBuffer {
	return &OutputBuffer{mode: mode}
}

// SetMode changes the telnet mode.
func (o *OutputBuffer) SetMode(mode TelnetMode) {
	o.mode = mode
}

// Receive appends data to the buffer and returns complete lines.
func (o *OutputBuffer) Receive(data []byte) []string {
	o.buffer.Write(data)
	o.newData = true
	buf := o.buffer.Bytes()
	var lines []string
	last := 0

	for i := 0; i < len(buf); i++ {
		// Handle \r\n or \n\r sequences
		if i+1 < len(buf) {
			if (buf[i] == '\r' && buf[i+1] == '\n') || (buf[i] == '\n' && buf[i+1] == '\r') {
				lines = append(lines, string(buf[last:i]))
				last = i + 2
				i++
				continue
			}
		}
		// Handle standalone \n
		if buf[i] == '\n' {
			lines = append(lines, string(buf[last:i]))
			last = i + 1
		}
	}

	// Keep remaining data in buffer
	if last > 0 {
		remaining := buf[last:]
		o.buffer.Reset()
		o.buffer.Write(remaining)
	}

	return lines
}

// Prompt returns any pending (unterminated) text.
// If consume is true, the buffer is cleared.
func (o *OutputBuffer) Prompt(consume bool) string {
	if o.buffer.Len() == 0 {
		return ""
	}
	text := o.buffer.String()
	if consume {
		o.buffer.Reset()
		o.newData = false
	}
	return text
}

// HasNewData reports whether the buffer has received new data since last prompt flush.
func (o *OutputBuffer) HasNewData() bool {
	return o.newData
}

// InputSent should be called when the user sends input.
// In unterminated prompt mode, this clears the buffer since the prompt
// will be reprinted by the server after echoing the input.
func (o *OutputBuffer) InputSent() {
	if o.mode == TelnetModeUnterminated {
		o.buffer.Reset()
		o.newData = false
	}
}

// Len returns the number of bytes in the buffer.
func (o *OutputBuffer) Len() int {
	return o.buffer.Len()
}

// Clear resets the buffer and mode.
func (o *OutputBuffer) Clear() {
	o.buffer.Reset()
	o.mode = TelnetModeUnterminated
	o.newData = false
}

// defaultCompatibility returns the default compatibility table for MUD clients.
func defaultCompatibility() CompatibilityTable {
	t := NewCompatibilityTable()
	t.Support(OptMCCP2)
	t.Support(OptMCCP3)
	t.Support(OptEOR)
	t.Support(OptEcho)
	t.Support(OptSGA)
	t.Support(OptNAWS)
	t.Support(OptTTYPE)
	t.Support(OptGMCP)
	t.Support(OptLinemode)
	return t
}
