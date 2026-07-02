package network

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/mmcdole/rune/version"
)

// mttsPlain is the expected MTTS bitvector for a non-TLS connection:
// ANSI + VT100 + UTF-8 + 256 COLORS + TRUECOLOR + MNES.
const mttsPlain = mttsANSI | mttsVT100 | mttsUTF8 | mtts256Colors | mttsTruecolor | mttsMNES

func ttypeIS(answer string) []byte {
	return subnegFrame(OptTTYPE, append([]byte{CmdIS}, []byte(answer)...))
}

// TestTTYPECycle verifies the MTTS terminal-type cycle: client name,
// then terminal, then "MTTS <bits>" repeated on further SENDs.
func TestTTYPECycle(t *testing.T) {
	h := newHandshake(false, 80, 24)
	send := []byte{CmdSEND}

	want := [][]byte{
		ttypeIS("RUNE"),
		ttypeIS("XTERM"),
		ttypeIS("MTTS " + strconv.Itoa(mttsPlain)),
		ttypeIS("MTTS " + strconv.Itoa(mttsPlain)), // stays on MTTS
	}
	for i, expected := range want {
		frames := h.onSubnegotiation(OptTTYPE, send)
		if len(frames) != 1 || !bytes.Equal(frames[0], expected) {
			t.Fatalf("TTYPE SEND #%d: got %v, want %v", i+1, frames, expected)
		}
	}
}

// TestTTYPEReportsTLSBit verifies the MTTS SSL bit is set only on TLS
// connections - the bits must reflect real capabilities.
func TestTTYPEReportsTLSBit(t *testing.T) {
	h := newHandshake(true, 80, 24)
	send := []byte{CmdSEND}

	h.onSubnegotiation(OptTTYPE, send) // name
	h.onSubnegotiation(OptTTYPE, send) // terminal
	frames := h.onSubnegotiation(OptTTYPE, send)

	want := ttypeIS("MTTS " + strconv.Itoa(mttsPlain|mttsTLS))
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("TLS MTTS: got %v, want %v", frames, want)
	}
}

// TestTTYPEIgnoresNonSend verifies garbage subnegotiations produce no reply.
func TestTTYPEIgnoresNonSend(t *testing.T) {
	h := newHandshake(false, 80, 24)
	if frames := h.onSubnegotiation(OptTTYPE, []byte{CmdIS, 'x'}); frames != nil {
		t.Fatalf("expected no reply to TTYPE IS, got %v", frames)
	}
	if frames := h.onSubnegotiation(OptTTYPE, nil); frames != nil {
		t.Fatalf("expected no reply to empty TTYPE, got %v", frames)
	}
}

// TestNAWSReportsSizeOnDO verifies DO NAWS gets an immediate
// big-endian size report and resizes re-send while active.
func TestNAWSReportsSizeOnDO(t *testing.T) {
	h := newHandshake(false, 120, 40)

	frames := h.onNegotiation(CmdDO, OptNAWS)
	want := subnegFrame(OptNAWS, []byte{0, 120, 0, 40})
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("DO NAWS: got %v, want %v", frames, want)
	}

	// Resize while active -> immediate report, multi-byte width
	frame := h.setWindowSize(300, 50)
	want = subnegFrame(OptNAWS, []byte{1, 44, 0, 50}) // 300 = 0x012C
	if !bytes.Equal(frame, want) {
		t.Fatalf("resize report: got %v, want %v", frame, want)
	}

	// DONT NAWS -> resizes go quiet
	h.onNegotiation(CmdDONT, OptNAWS)
	if frame := h.setWindowSize(100, 30); frame != nil {
		t.Fatalf("expected no NAWS report after DONT, got %v", frame)
	}
}

// TestNAWSEscapesIACWidth verifies a size byte of 255 is IAC-escaped
// inside the subnegotiation (RFC 1073 + telnet framing).
func TestNAWSEscapesIACWidth(t *testing.T) {
	h := newHandshake(false, 255, 24)
	frames := h.onNegotiation(CmdDO, OptNAWS)

	// Payload [0, 255, 0, 24] -> 255 doubled on the wire
	want := []byte{CmdIAC, CmdSB, OptNAWS, 0, 255, 255, 0, 24, CmdIAC, CmdSE}
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("IAC width escaping: got %v, want %v", frames, want)
	}
}

// TestNAWSDefaultsWhenSizeUnknown verifies a connection that has never
// seen a resize reports 80x24 instead of 0x0.
func TestNAWSDefaultsWhenSizeUnknown(t *testing.T) {
	h := newHandshake(false, 0, 0)
	frames := h.onNegotiation(CmdDO, OptNAWS)
	want := subnegFrame(OptNAWS, []byte{0, 80, 0, 24})
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("default size: got %v, want %v", frames, want)
	}
}

// TestCharsetAcceptsUTF8 verifies REQUEST handling: UTF-8 accepted
// (case-insensitively), otherwise rejected, TTABLE prefix skipped.
func TestCharsetAcceptsUTF8(t *testing.T) {
	h := newHandshake(false, 80, 24)
	accepted := subnegFrame(OptCharset, append([]byte{charsetAccepted}, []byte("UTF-8")...))
	rejected := subnegFrame(OptCharset, []byte{charsetRejected})

	cases := []struct {
		name    string
		request []byte
		want    []byte
	}{
		{"utf-8 offered", append([]byte{charsetRequest}, []byte(";UTF-8;ISO-8859-1")...), accepted},
		{"utf-8 lowercase", append([]byte{charsetRequest}, []byte(";utf-8")...), accepted},
		{"no utf-8", append([]byte{charsetRequest}, []byte(";ISO-8859-1;CP437")...), rejected},
		{"ttable prefix", append([]byte{charsetRequest}, append([]byte("[TTABLE]\x01"), []byte(";UTF-8")...)...), accepted},
		{"empty list", []byte{charsetRequest, ';'}, rejected},
	}
	for _, c := range cases {
		frames := h.onSubnegotiation(OptCharset, c.request)
		if len(frames) != 1 || !bytes.Equal(frames[0], c.want) {
			t.Errorf("%s: got %v, want %v", c.name, frames, c.want)
		}
	}

	// Non-REQUEST subnegotiation -> no reply
	if frames := h.onSubnegotiation(OptCharset, []byte{charsetAccepted}); frames != nil {
		t.Errorf("expected no reply to ACCEPTED, got %v", frames)
	}
}

// TestEnvironSendAllVariables verifies an empty SEND returns every
// MNES variable with values.
func TestEnvironSendAllVariables(t *testing.T) {
	h := newHandshake(false, 80, 24)
	frames := h.onSubnegotiation(OptNewEnviron, []byte{environSEND})
	if len(frames) != 1 {
		t.Fatalf("expected one IS reply, got %v", frames)
	}

	var payload []byte
	payload = append(payload, environIS)
	appendVar := func(name, value string) {
		payload = append(payload, environVAR)
		payload = append(payload, []byte(name)...)
		payload = append(payload, environVALUE)
		payload = append(payload, []byte(value)...)
	}
	appendVar("CLIENT_NAME", "RUNE")
	appendVar("CLIENT_VERSION", version.Number)
	appendVar("CHARSET", "UTF-8")
	appendVar("MTTS", strconv.Itoa(mttsPlain))
	appendVar("TERMINAL_TYPE", "XTERM")
	want := subnegFrame(OptNewEnviron, payload)

	if !bytes.Equal(frames[0], want) {
		t.Fatalf("SEND-all reply:\n got %v\nwant %v", frames[0], want)
	}
}

// TestEnvironSendSpecificVariables verifies requested variables are
// echoed with the type they were requested as, and unknown variables
// come back without a VALUE.
func TestEnvironSendSpecificVariables(t *testing.T) {
	h := newHandshake(false, 80, 24)

	request := []byte{environSEND, environVAR}
	request = append(request, []byte("CLIENT_NAME")...)
	request = append(request, environUSERVAR)
	request = append(request, []byte("MTTS")...)
	request = append(request, environVAR)
	request = append(request, []byte("BOGUS")...)

	frames := h.onSubnegotiation(OptNewEnviron, request)
	if len(frames) != 1 {
		t.Fatalf("expected one IS reply, got %v", frames)
	}

	var payload []byte
	payload = append(payload, environIS, environVAR)
	payload = append(payload, []byte("CLIENT_NAME")...)
	payload = append(payload, environVALUE)
	payload = append(payload, []byte("RUNE")...)
	payload = append(payload, environUSERVAR)
	payload = append(payload, []byte("MTTS")...)
	payload = append(payload, environVALUE)
	payload = append(payload, []byte(strconv.Itoa(mttsPlain))...)
	payload = append(payload, environVAR)
	payload = append(payload, []byte("BOGUS")...) // no VALUE: undefined
	want := subnegFrame(OptNewEnviron, payload)

	if !bytes.Equal(frames[0], want) {
		t.Fatalf("SEND-specific reply:\n got %v\nwant %v", frames[0], want)
	}
}

// TestGMCPSplit verifies package/payload separation.
func TestGMCPSplit(t *testing.T) {
	cases := []struct {
		in, pkg, payload string
	}{
		{`Char.Vitals {"hp":10}`, "Char.Vitals", `{"hp":10}`},
		{"Core.Ping", "Core.Ping", ""},
		{"  Room.Info   {}  ", "Room.Info", "{}"},
		{"", "", ""},
	}
	for _, c := range cases {
		pkg, payload := splitGMCP([]byte(c.in))
		if pkg != c.pkg || payload != c.payload {
			t.Errorf("splitGMCP(%q) = (%q, %q), want (%q, %q)", c.in, pkg, payload, c.pkg, c.payload)
		}
	}
}
