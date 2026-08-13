package network

import "fmt"

// EffectKind identifies one ordered consequence of a Telnet event.
type EffectKind uint8

const (
	// EffectSendFrame is a Telnet frame that must be queued before processing
	// later effects from the batch.
	EffectSendFrame EffectKind = iota
	EffectServerData
	EffectGA
	EffectEOR
	EffectGMCPEnabled
	EffectGMCPMessage
)

// Effect is emitted synchronously by Protocol.Process. Data contains bytes for
// outbound frames and server text; Package and Payload describe a GMCP message.
type Effect struct {
	Kind    EffectKind
	Data    []byte
	Package string
	Payload string
}

// Protocol interprets parser events and owns the Telnet state used by Session.
// A Protocol belongs to one connection and is called only by Session's event
// loop. Its synchronous emitter keeps Lua callbacks and the sends they produce
// in wire order with the rest of the batch.
type Protocol struct {
	handshake  *handshake
	gmcpLocal  bool
	gmcpRemote bool
	localEcho  bool
}

// NewProtocol creates the protocol state for one connection. Transport
// security is supplied by each EventBatch before identity replies are built.
func NewProtocol(width, height int) *Protocol {
	return &Protocol{
		handshake: newHandshake(false, width, height),
		localEcho: true,
	}
}

// Process interprets a complete parser batch in wire order. emit is invoked
// synchronously for each externally visible effect. Returning false from emit
// stops the batch, allowing Session to abandon stale or failed connections.
func (p *Protocol) Process(batch EventBatch, emit func(Effect) bool) bool {
	p.handshake.setSecure(batch.Secure)

	for _, event := range batch.Events {
		switch event.Kind {
		case TelnetEventDataSend:
			if !emit(Effect{Kind: EffectSendFrame, Data: event.Data}) {
				return false
			}

		case TelnetEventDataReceive:
			if !emit(Effect{Kind: EffectServerData, Data: event.Data}) {
				return false
			}

		case TelnetEventIAC:
			var kind EffectKind
			switch event.Command {
			case CmdGA:
				kind = EffectGA
			case CmdEOR:
				kind = EffectEOR
			default:
				continue
			}
			if !emit(Effect{Kind: kind}) {
				return false
			}

		case TelnetEventNegotiation:
			gmcpEnabled := p.applyNegotiation(event.Command, event.Option)
			for _, frame := range p.handshake.onNegotiation(event.Command, event.Option) {
				if !emit(Effect{Kind: EffectSendFrame, Data: frame}) {
					return false
				}
			}
			if gmcpEnabled {
				if !emit(Effect{Kind: EffectGMCPEnabled}) {
					return false
				}
			}

		case TelnetEventSubnegotiation:
			if event.Option == OptGMCP {
				if !p.GMCPActive() {
					continue
				}
				pkg, payload := splitGMCP(event.Data)
				if pkg != "" && !emit(Effect{Kind: EffectGMCPMessage, Package: pkg, Payload: payload}) {
					return false
				}
				continue
			}
			if event.Option == OptMCCP2 || event.Option == OptMCCP3 {
				continue
			}
			for _, frame := range p.handshake.onSubnegotiation(event.Option, event.Data) {
				if !emit(Effect{Kind: EffectSendFrame, Data: frame}) {
					return false
				}
			}
		}
	}
	return true
}

// GMCPActive reports whether the current negotiation permits GMCP messages.
func (p *Protocol) GMCPActive() bool {
	return p.gmcpLocal || p.gmcpRemote
}

// LocalEchoEnabled reports whether submitted commands should be shown locally.
func (p *Protocol) LocalEchoEnabled() bool {
	return p.localEcho
}

// GMCPFrame builds an outbound GMCP frame when GMCP is active.
func (p *Protocol) GMCPFrame(pkg, data string) ([]byte, error) {
	if !p.GMCPActive() {
		return nil, fmt.Errorf("GMCP not negotiated on this connection")
	}
	payload := pkg
	if data != "" {
		payload += " " + data
	}
	return subnegFrame(OptGMCP, []byte(payload)), nil
}

// SetWindowSize records the terminal size and returns a NAWS report when the
// server has enabled that option.
func (p *Protocol) SetWindowSize(width, height int) []byte {
	return p.handshake.setWindowSize(width, height)
}

func (p *Protocol) applyNegotiation(command, option byte) (gmcpEnabled bool) {
	wasGMCPActive := p.GMCPActive()
	switch option {
	case OptEcho:
		switch command {
		case CmdWILL:
			p.localEcho = false
		case CmdWONT:
			p.localEcho = true
		}

	case OptGMCP:
		switch command {
		case CmdWILL:
			p.gmcpRemote = true
		case CmdWONT:
			p.gmcpRemote = false
		case CmdDO:
			p.gmcpLocal = true
		case CmdDONT:
			p.gmcpLocal = false
		}
	}
	return option == OptGMCP && !wasGMCPActive && p.GMCPActive()
}
