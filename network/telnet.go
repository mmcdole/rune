// Package network provides the telnet protocol layer for the client:
// stream parsing, option negotiation, and connection management.
package network

// Telnet command codes.
const (
	CmdIAC  byte = 255 // Interpret As Command
	CmdWILL byte = 251 // Will use option
	CmdWONT byte = 252 // Won't use option
	CmdDO   byte = 253 // Do use option
	CmdDONT byte = 254 // Don't use option
	CmdNOP  byte = 241 // No operation
	CmdDM   byte = 242 // Data mark
	CmdAYT  byte = 246 // Are you there
	CmdSB   byte = 250 // Subnegotiation begin
	CmdSE   byte = 240 // Subnegotiation end
	CmdIS   byte = 0   // Subnegotiation IS
	CmdSEND byte = 1   // Subnegotiation SEND
	CmdGA   byte = 249 // Go ahead
	CmdEOR  byte = 239 // End of record
)

// Telnet option codes.
const (
	OptEcho       byte = 1
	OptSGA        byte = 3 // Suppress Go Ahead
	OptTTYPE      byte = 24
	OptEOR        byte = 25
	OptNAWS       byte = 31
	OptLinemode   byte = 34
	OptNewEnviron byte = 39
	OptCharset    byte = 42
	OptMSSP       byte = 70
	OptMCCP2      byte = 86
	OptMCCP3      byte = 87
	OptZMP        byte = 93
	OptGMCP       byte = 201
)

// TelnetEventKind represents the type of telnet event.
type TelnetEventKind int

const (
	// TelnetEventData is received application data.
	TelnetEventData TelnetEventKind = iota
	// TelnetEventSendFrame is a negotiation reply the client must send.
	TelnetEventSendFrame
	TelnetEventIAC
	TelnetEventNegotiation
	TelnetEventSubnegotiation
	// TelnetEventCompressStart marks the start of a negotiated MCCP2 stream.
	// Data holds the bytes that followed it in the same read, possibly none;
	// they and every later byte are compressed.
	TelnetEventCompressStart
)

// TelnetEvent carries parser output.
type TelnetEvent struct {
	Kind    TelnetEventKind
	Command byte   // For IAC, Negotiation
	Option  byte   // For Negotiation, Subnegotiation
	Data    []byte // For Data, SendFrame, Subnegotiation, CompressStart
}

// compatibilityEntry represents the negotiation state for a single telnet option.
type compatibilityEntry struct {
	Local       bool // We support this option locally (us -> them)
	Remote      bool // We support this option remotely (them -> us)
	LocalState  bool // Option currently enabled locally
	RemoteState bool // Option currently enabled remotely
}

// compatibilityTable tracks negotiation state for all 256 telnet options.
type compatibilityTable [256]compatibilityEntry

func (t *compatibilityTable) supportLocal(option byte) {
	t[option].Local = true
}

func (t *compatibilityTable) supportRemote(option byte) {
	t[option].Remote = true
}

func (t *compatibilityTable) support(option byte) {
	t.supportLocal(option)
	t.supportRemote(option)
}

// Parser is a telnet protocol parser.
type Parser struct {
	options compatibilityTable
	buffer  []byte
}

// NewParser creates a parser with the default client compatibility table.
func NewParser() *Parser {
	return newParser(defaultCompatibility())
}

// newParser creates a parser over an explicit compatibility table; tests use
// it to exercise non-default negotiation states.
func newParser(table compatibilityTable) *Parser {
	return &Parser{
		options: table,
		buffer:  make([]byte, 0, 128),
	}
}

func (p *Parser) Receive(data []byte) []TelnetEvent {
	p.buffer = append(p.buffer, data...)
	return p.process()
}

// escapeIAC doubles IAC bytes for outbound data.
// Example: [255, 1, 6, 2] -> [255, 255, 1, 6, 2]
func escapeIAC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		out = append(out, b)
		if b == CmdIAC {
			out = append(out, CmdIAC)
		}
	}
	return out
}

// unescapeIAC collapses doubled IAC bytes in received data.
// Example: [255, 255, 1, 6, 2] -> [255, 1, 6, 2]
func unescapeIAC(data []byte) []byte {
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
	remaining []byte
}

func (p *Parser) process() []TelnetEvent {
	var out []TelnetEvent
	events := p.extract()
	for i := 0; i < len(events); {
		ev := events[i]
		if ev.kind == evNone {
			end := i + 1
			total := len(ev.buf)
			for end < len(events) && events[end].kind == evNone {
				total += len(events[end].buf)
				end++
			}

			data := ev.buf
			if end > i+1 {
				data = make([]byte, 0, total)
				for _, part := range events[i:end] {
					data = append(data, part.buf...)
				}
			}
			if len(data) > 0 {
				out = append(out, TelnetEvent{Kind: TelnetEventData, Data: data})
			}
			i = end
			continue
		}

		switch ev.kind {
		case evIAC, evNeg:
			out = append(out, p.processCommand(ev.buf)...)
		case evSub:
			out = append(out, p.processSub(ev.buf, ev.remaining)...)
		}
		i++
	}
	return out
}

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
				// Double IAC encodes one literal 255 data byte.
				res = append(res, parsedSlice{kind: evNone, buf: buf[cmdBegin:i]})
				state = stateNormal
				cmdBegin = i + 1
			case CmdSB:
				state = stateSub
			case CmdWILL, CmdWONT, CmdDO, CmdDONT:
				state = stateNeg
			default:
				res = append(res, parsedSlice{kind: evIAC, buf: buf[cmdBegin : i+1]})
				state = stateNormal
				cmdBegin = i + 1
			}

		case stateNeg:
			res = append(res, parsedSlice{kind: evNeg, buf: buf[cmdBegin : i+1]})
			state = stateNormal
			cmdBegin = i + 1

		case stateSub:
			subOpt = val
			state = stateSubOpt

		case stateSubOpt:
			if val == CmdIAC {
				state = stateSubIAC
			}

		case stateSubIAC:
			if val == CmdSE {
				// Negotiated MCCP2: everything after SE is compressed, so
				// parsing stops here. An unsolicited MCCP2 subnegotiation is
				// dropped without discarding the plaintext behind it, and
				// MCCP3 (client->server compression, always refused) never
				// changes the read path.
				mccp2 := p.options[OptMCCP2]
				if subOpt == OptMCCP2 && mccp2.Remote && mccp2.RemoteState {
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
				// Escaped IAC within subnegotiation
			} else {
				state = stateSubOpt
			}
		}
	}

	if cmdBegin < len(buf) {
		switch state {
		case stateSub, stateSubOpt, stateSubIAC, stateIAC, stateNeg:
			p.buffer = append(p.buffer, buf[cmdBegin:]...)
		default:
			res = append(res, parsedSlice{kind: evNone, buf: buf[cmdBegin:]})
		}
	}

	return res
}

func (p *Parser) processCommand(buf []byte) []TelnetEvent {
	var out []TelnetEvent

	if len(buf) >= 2 && buf[0] == CmdIAC {
		cmd := buf[1]
		if cmd != CmdSE {
			if len(buf) == 2 {
				out = append(out, TelnetEvent{Kind: TelnetEventIAC, Command: cmd})
			} else if len(buf) == 3 {
				out = append(out, p.processNegotiation(buf[1], buf[2])...)
			}
		}
	}

	return out
}

// processSub interprets one complete IAC SB ... IAC SE slice; extract only
// emits evSub after IAC SE, so buf always has that shape.
func (p *Parser) processSub(buf, remaining []byte) []TelnetEvent {
	// Accept subnegotiations for options enabled on either side.
	// Server-offered protocols (GMCP, MCCP) are enabled remotely
	// (WILL from the server, DO from us); client-answered options
	// (TTYPE, NAWS) are enabled locally (DO from the server, WILL
	// from us). Both directions legitimately carry subnegotiations.
	opt := buf[2]
	entry := p.options[opt]
	localOn := entry.Local && entry.LocalState
	remoteOn := entry.Remote && entry.RemoteState
	if !localOn && !remoteOn {
		return nil
	}

	if opt == OptMCCP2 {
		return []TelnetEvent{{Kind: TelnetEventCompressStart, Data: remaining}}
	}
	return []TelnetEvent{{
		Kind:   TelnetEventSubnegotiation,
		Option: opt,
		Data:   unescapeIAC(buf[3 : len(buf)-2]),
	}}
}

func (p *Parser) processNegotiation(command, opt byte) []TelnetEvent {
	entry := &p.options[opt]
	var responses []TelnetEvent

	switch command {
	case CmdWILL:
		if entry.Remote && !entry.RemoteState {
			entry.RemoteState = true
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventSendFrame,
				Data: []byte{CmdIAC, CmdDO, opt},
			})
			responses = append(responses, TelnetEvent{
				Kind:    TelnetEventNegotiation,
				Command: command,
				Option:  opt,
			})
		} else if !entry.Remote {
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventSendFrame,
				Data: []byte{CmdIAC, CmdDONT, opt},
			})
		}

	case CmdWONT:
		if entry.RemoteState {
			entry.RemoteState = false
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventSendFrame,
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
			// DO enables our side only; the remote side needs the
			// server's own WILL.
			entry.LocalState = true
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventSendFrame,
				Data: []byte{CmdIAC, CmdWILL, opt},
			})
			responses = append(responses, TelnetEvent{
				Kind:    TelnetEventNegotiation,
				Command: command,
				Option:  opt,
			})
		} else if !entry.Local {
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventSendFrame,
				Data: []byte{CmdIAC, CmdWONT, opt},
			})
		}

	case CmdDONT:
		if entry.LocalState {
			entry.LocalState = false
			responses = append(responses, TelnetEvent{
				Kind: TelnetEventSendFrame,
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

// defaultCompatibility advertises ONLY options the client actually
// implements. Advertising an option we cannot honor is worse than
// refusing it: agreeing to MCCP means the server starts sending
// compressed data we cannot decompress, agreeing to TTYPE/NAWS means
// the server waits for subnegotiation replies that never come. Add
// options here only together with their implementation.
func defaultCompatibility() compatibilityTable {
	var t compatibilityTable
	t.supportRemote(OptEcho)      // WILL/WONT ECHO controls local echo (protocol.go); we never echo to the server
	t.support(OptSGA)             // Suppress Go Ahead: full-duplex handshake
	t.supportRemote(OptEOR)       // End of Record: servers mark prompts; we never send them
	t.supportLocal(OptTTYPE)      // Terminal type + MTTS cycle (negotiate.go)
	t.supportLocal(OptNAWS)       // Window size reports (negotiate.go, protocol.go)
	t.support(OptCharset)         // UTF-8 charset negotiation (negotiate.go)
	t.supportLocal(OptNewEnviron) // MNES client identification (negotiate.go)
	t.supportRemote(OptMCCP2)     // Server->client zlib compression (client.go)
	t.support(OptGMCP)            // Out-of-band JSON messages (protocol.go, Lua rune.gmcp)
	return t
}
