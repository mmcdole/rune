package network

import (
	"bytes"
	"strconv"
	"strings"
	"sync"

	"github.com/mmcdole/rune/version"
)

// CHARSET subnegotiation commands (RFC 2066).
const (
	charsetRequest  byte = 1
	charsetAccepted byte = 2
	charsetRejected byte = 3
)

// NEW-ENVIRON subnegotiation commands and variable types (RFC 1572).
const (
	environIS      byte = 0
	environSEND    byte = 1
	environVAR     byte = 0
	environVALUE   byte = 1
	environESC     byte = 2
	environUSERVAR byte = 3
)

// MTTS capability bits (https://tintin.mudhalla.net/protocols/mtts/).
const (
	mttsANSI      = 1
	mttsVT100     = 2
	mttsUTF8      = 4
	mtts256Colors = 8
	mttsTruecolor = 256
	mttsMNES      = 512
	mttsTLS       = 2048
)

// subnegFrame builds IAC SB <option> <escaped payload> IAC SE.
func subnegFrame(option byte, payload []byte) []byte {
	escaped := EscapeIAC(payload)
	buf := make([]byte, 0, len(escaped)+5)
	buf = append(buf, CmdIAC, CmdSB, option)
	buf = append(buf, escaped...)
	buf = append(buf, CmdIAC, CmdSE)
	return buf
}

// handshake answers the telnet identity options: TTYPE/MTTS, NAWS,
// CHARSET, and NEW-ENVIRON/MNES. It is a pure responder - methods
// take parser events and return raw frames to write - so it is fully
// testable without sockets. A mutex guards the NAWS/TTYPE state:
// readLoop drives negotiation while SetWindowSize arrives from the
// session goroutine.
type handshake struct {
	mu         sync.Mutex
	tls        bool
	width      int
	height     int
	ttypeIndex int
	nawsActive bool
}

func newHandshake(tls bool, width, height int) *handshake {
	return &handshake{tls: tls, width: width, height: height}
}

// mtts computes the MTTS bitvector. Honesty rule: every bit here must
// reflect a real client capability.
func (h *handshake) mtts() int {
	bits := mttsANSI | mttsVT100 | mttsUTF8 | mtts256Colors | mttsTruecolor | mttsMNES
	if h.tls {
		bits |= mttsTLS
	}
	return bits
}

// clientName is the uppercase client name MTTS/MNES expect.
func clientName() string {
	return strings.ToUpper(version.Name)
}

// onNegotiation reacts to option state changes that require an
// immediate client subnegotiation. Returns frames to write.
func (h *handshake) onNegotiation(cmd, opt byte) [][]byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	if opt == OptNAWS {
		switch cmd {
		case CmdDO:
			h.nawsActive = true
			return [][]byte{h.nawsFrameLocked()}
		case CmdDONT:
			h.nawsActive = false
		}
	}
	return nil
}

// onSubnegotiation answers a server subnegotiation request.
// Returns frames to write (nil when the option is not ours to answer).
func (h *handshake) onSubnegotiation(opt byte, data []byte) [][]byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch opt {
	case OptTTYPE:
		return h.ttypeReplyLocked(data)
	case OptCharset:
		return charsetReply(data)
	case OptNewEnviron:
		return h.environReplyLocked(data)
	}
	return nil
}

// setWindowSize records the new size and returns a NAWS frame to send
// if the option is currently active, nil otherwise.
func (h *handshake) setWindowSize(width, height int) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.width, h.height = width, height
	if !h.nawsActive {
		return nil
	}
	return h.nawsFrameLocked()
}

// nawsFrameLocked builds the RFC 1073 window-size report.
// Caller holds h.mu.
func (h *handshake) nawsFrameLocked() []byte {
	w, ht := h.width, h.height
	if w <= 0 {
		w = 80
	}
	if ht <= 0 {
		ht = 24
	}
	payload := []byte{byte(w >> 8), byte(w), byte(ht >> 8), byte(ht)}
	return subnegFrame(OptNAWS, payload)
}

// ttypeReplyLocked answers TTYPE SEND per the MTTS cycle:
// client name, then terminal type, then "MTTS <bits>" (repeated).
// Caller holds h.mu.
func (h *handshake) ttypeReplyLocked(data []byte) [][]byte {
	if len(data) < 1 || data[0] != CmdSEND {
		return nil
	}

	var answer string
	switch h.ttypeIndex {
	case 0:
		answer = clientName()
	case 1:
		answer = "XTERM"
	default:
		answer = "MTTS " + strconv.Itoa(h.mtts())
	}
	if h.ttypeIndex < 2 {
		h.ttypeIndex++
	}

	payload := append([]byte{CmdIS}, []byte(answer)...)
	return [][]byte{subnegFrame(OptTTYPE, payload)}
}

// charsetReply answers a CHARSET REQUEST: ACCEPTED UTF-8 when offered,
// REJECTED otherwise. Handles the optional [TTABLE] prefix by refusing
// translation tables (we only ever accept UTF-8 verbatim).
func charsetReply(data []byte) [][]byte {
	if len(data) < 1 || data[0] != charsetRequest {
		return nil
	}
	rest := data[1:]

	// RFC 2066: REQUEST may carry "[TTABLE]" plus a version byte
	// before the charset list. Skip it; we never accept a ttable.
	if bytes.HasPrefix(rest, []byte("[TTABLE]")) {
		rest = rest[len("[TTABLE]"):]
		if len(rest) < 1 {
			return [][]byte{subnegFrame(OptCharset, []byte{charsetRejected})}
		}
		rest = rest[1:] // version byte
	}
	if len(rest) < 2 {
		return [][]byte{subnegFrame(OptCharset, []byte{charsetRejected})}
	}

	sep := rest[0]
	for _, cs := range bytes.Split(rest[1:], []byte{sep}) {
		if strings.EqualFold(string(cs), "UTF-8") {
			payload := append([]byte{charsetAccepted}, []byte("UTF-8")...)
			return [][]byte{subnegFrame(OptCharset, payload)}
		}
	}
	return [][]byte{subnegFrame(OptCharset, []byte{charsetRejected})}
}

// environValueLocked returns the MNES value for a variable, or
// ok=false for variables we do not define. Caller holds h.mu.
func (h *handshake) environValueLocked(name string) (string, bool) {
	switch strings.ToUpper(name) {
	case "CLIENT_NAME":
		return clientName(), true
	case "CLIENT_VERSION":
		return version.Number, true
	case "CHARSET":
		return "UTF-8", true
	case "MTTS":
		return strconv.Itoa(h.mtts()), true
	case "TERMINAL_TYPE":
		return "XTERM", true
	}
	return "", false
}

// mnesVars is the set (and reply order) of variables offered when the
// server sends an empty SEND, meaning "everything you have".
var mnesVars = []string{"CLIENT_NAME", "CLIENT_VERSION", "CHARSET", "MTTS", "TERMINAL_TYPE"}

// environReplyLocked answers NEW-ENVIRON SEND per RFC 1572 / MNES:
// requested variables get VALUE entries (echoing the VAR/USERVAR type
// they were requested with); unknown ones are echoed without a VALUE;
// an empty SEND gets every variable we define. Caller holds h.mu.
func (h *handshake) environReplyLocked(data []byte) [][]byte {
	if len(data) < 1 || data[0] != environSEND {
		return nil
	}

	payload := []byte{environIS}

	appendVar := func(kind byte, name string) {
		payload = append(payload, kind)
		payload = append(payload, []byte(name)...)
		if value, ok := h.environValueLocked(name); ok {
			payload = append(payload, environVALUE)
			payload = append(payload, []byte(value)...)
		}
	}

	requested := parseEnvironNames(data[1:])
	if len(requested) == 0 {
		for _, name := range mnesVars {
			appendVar(environVAR, name)
		}
	} else {
		for _, req := range requested {
			appendVar(req.kind, req.name)
		}
	}

	return [][]byte{subnegFrame(OptNewEnviron, payload)}
}

type environName struct {
	kind byte // environVAR or environUSERVAR, echoed back in the reply
	name string
}

// parseEnvironNames decodes the VAR/USERVAR name list of a SEND
// request, honoring ESC-quoted bytes.
func parseEnvironNames(data []byte) []environName {
	var names []environName
	var current *environName

	flush := func() {
		if current != nil && current.name != "" {
			names = append(names, *current)
		}
		current = nil
	}

	for i := 0; i < len(data); i++ {
		switch data[i] {
		case environVAR, environUSERVAR:
			flush()
			current = &environName{kind: data[i]}
		case environESC:
			if current != nil && i+1 < len(data) {
				i++
				current.name += string(data[i])
			}
		default:
			if current != nil {
				current.name += string(data[i])
			}
		}
	}
	flush()
	return names
}
