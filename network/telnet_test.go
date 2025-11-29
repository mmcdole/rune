package network

import (
	"bytes"
	"testing"
)

// Helper to build a subnegotiation sequence
func buildSubneg(opt byte, payload []byte) []byte {
	escaped := EscapeIAC(payload)
	out := make([]byte, 0, 5+len(escaped))
	out = append(out, CmdIAC, CmdSB, opt)
	out = append(out, escaped...)
	out = append(out, CmdIAC, CmdSE)
	return out
}

// eventKinds extracts just the event kinds for comparison
func eventKinds(events []TelnetEvent) []TelnetEventKind {
	kinds := make([]TelnetEventKind, len(events))
	for i, ev := range events {
		kinds[i] = ev.Kind
	}
	return kinds
}

// Ensure DO negotiations split across TCP reads are handled and replied to.
func TestParserHandlesSplitDoNegotiation(t *testing.T) {
	parser := NewParser(DefaultCompatibility())

	// First chunk ends mid-command: IAC DO (missing option) - should emit nothing.
	events := parser.Receive([]byte{CmdIAC, CmdDO})
	if len(events) != 0 {
		t.Fatalf("expected no events yet, got %v", events)
	}

	// Second chunk provides the option byte; we should reply WILL NAWS.
	events = parser.Receive([]byte{OptNAWS})
	var reply []byte
	for _, ev := range events {
		if ev.Kind == TelnetEventDataSend {
			reply = ev.Data
			break
		}
	}
	if reply == nil {
		t.Fatalf("expected a negotiation reply, got none")
	}
	expected := []byte{CmdIAC, CmdWILL, OptNAWS}
	if !bytes.Equal(reply, expected) {
		t.Fatalf("unexpected reply: want %v got %v", expected, reply)
	}
}

func TestParser(t *testing.T) {
	parser := NewParserDefault()
	parser.Options.SupportLocal(OptGMCP)
	parser.Options.SupportLocal(OptMCCP2)

	// Test Will() returns send event when locally supported
	ev := parser.Will(OptGMCP)
	if ev == nil {
		t.Fatal("Will(GMCP) should return event")
	}
	if ev.Kind != TelnetEventDataSend {
		t.Errorf("Will should return DataSend, got %v", ev.Kind)
	}

	ev = parser.Will(OptMCCP2)
	if ev == nil {
		t.Fatal("Will(MCCP2) should return event")
	}

	// Test receiving data with IAC GA
	events := parser.Receive(append([]byte("Hello, rust!"), CmdIAC, CmdGA))
	kinds := eventKinds(events)
	expected := []TelnetEventKind{TelnetEventDataReceive, TelnetEventIAC}
	if len(kinds) != len(expected) {
		t.Fatalf("Expected %d events, got %d", len(expected), len(kinds))
	}
	for i := range expected {
		if kinds[i] != expected[i] {
			t.Errorf("Event %d: expected %v, got %v", i, expected[i], kinds[i])
		}
	}

	// Test IAC DO GMCP - since we already called Will(GMCP), LocalState is true,
	// so receiving DO should produce no events (already enabled)
	events = parser.Receive([]byte{CmdIAC, CmdDO, OptGMCP})
	if len(events) != 0 {
		t.Errorf("Expected 0 events for DO GMCP (already enabled), got %d", len(events))
	}

	// Test IAC DO for unsupported option (200) + data
	events = parser.Receive(append([]byte{CmdIAC, CmdDO, 200}, []byte("Some random data")...))
	// Should get: DataSend (WONT) + DataReceive
	kinds = eventKinds(events)
	expectedKinds := []TelnetEventKind{TelnetEventDataSend, TelnetEventDataReceive}
	if len(kinds) != len(expectedKinds) {
		t.Fatalf("Expected %d events, got %d: %+v", len(expectedKinds), len(kinds), events)
	}

	// Test subnegotiation for GMCP
	gmcpData := buildSubneg(OptGMCP, []byte("Core.Hello {}"))
	events = parser.Receive(gmcpData)
	if len(events) != 1 || events[0].Kind != TelnetEventSubnegotiation {
		t.Errorf("Expected 1 Subnegotiation event, got %d events", len(events))
	}
	if events[0].Option != OptGMCP {
		t.Errorf("Expected option GMCP, got %d", events[0].Option)
	}
	if string(events[0].Data) != "Core.Hello {}" {
		t.Errorf("Expected payload 'Core.Hello {}', got '%s'", events[0].Data)
	}

	// Test subnegotiation + data + GA
	combined := append(append(gmcpData, []byte("Random text")...), CmdIAC, CmdGA)
	events = parser.Receive(combined)
	kinds = eventKinds(events)
	expectedKinds = []TelnetEventKind{TelnetEventSubnegotiation, TelnetEventDataReceive, TelnetEventIAC}
	if len(kinds) != len(expectedKinds) {
		t.Fatalf("Expected %d events, got %d", len(expectedKinds), len(kinds))
	}

	// Test MCCP2 subnegotiation with remaining data
	mccp2Data := append(buildSubneg(OptMCCP2, []byte(" ")), []byte("This is compressed data")...)
	mccp2Data = append(mccp2Data, CmdIAC, CmdGA)
	events = parser.Receive(mccp2Data)
	kinds = eventKinds(events)
	expectedKinds = []TelnetEventKind{TelnetEventSubnegotiation, TelnetEventDecompressImmediate}
	if len(kinds) != len(expectedKinds) {
		t.Fatalf("Expected %d events, got %d: %+v", len(expectedKinds), len(kinds), events)
	}

	// Test realistic data with EOR and WILL
	// "What is your password? " + IAC EOR + IAC WILL ECHO
	realData := []byte{
		87, 104, 97, 116, 32, 105, 115, 32, 121, 111, 117, 114, 32, 112, 97, 115, 115, 119, 111, 114,
		100, 63, 32, CmdIAC, CmdEOR, CmdIAC, CmdWILL, 1,
	}
	events = parser.Receive(realData)
	kinds = eventKinds(events)
	// DataReceive + IAC (EOR) + DataSend (response to WILL)
	if len(kinds) < 2 {
		t.Fatalf("Expected at least 2 events, got %d: %+v", len(kinds), events)
	}
}

func TestSubnegSeparateReceives(t *testing.T) {
	parser := NewParserWithCapacity(10)
	parser.Options.SupportLocal(OptGMCP)
	parser.Will(OptGMCP)

	// Receive start of subnegotiation
	events := parser.Receive(append(
		[]byte{CmdIAC, CmdSB, OptGMCP},
		[]byte("Option.Data { some: json, data: in, here: ! }")...,
	))
	if len(events) != 0 {
		t.Errorf("Expected 0 events for incomplete subneg, got %d", len(events))
	}

	// Receive more data
	events = parser.Receive([]byte("More.Data { some: json, data: in, here: ! }"))
	if len(events) != 0 {
		t.Errorf("Expected 0 events for still incomplete subneg, got %d", len(events))
	}

	// Complete first subneg and start second
	events = parser.Receive(append(
		append([]byte{CmdIAC, CmdSE}, CmdIAC, CmdSB, OptGMCP),
		[]byte("Option.Data { some: json, data: in, here: ! }")...,
	))
	if len(events) != 1 || events[0].Kind != TelnetEventSubnegotiation {
		t.Errorf("Expected 1 Subnegotiation event, got %d events", len(events))
	}

	// Complete second subneg
	events = parser.Receive(append(
		[]byte("More.Data { some: json, data: in, here: ! }"),
		CmdIAC, CmdSE,
	))
	if len(events) != 1 || events[0].Kind != TelnetEventSubnegotiation {
		t.Errorf("Expected 1 Subnegotiation event, got %d events", len(events))
	}
}

func TestSubnegUTF8Content(t *testing.T) {
	// Test that receiving a subnegotiation with embedded UTF-8 content works correctly,
	// even when the content includes a SE byte (0xF0 in wave emoji).
	parser := NewParserDefault()
	parser.Options.SupportLocal(OptGMCP)
	parser.Will(OptGMCP)

	// Wave emoji: 0xF0, 0x9F, 0x91, 0x8B - where 0xF0 matches SE
	waveEmoji := []byte{0xF0, 0x9F, 0x91, 0x8B}
	gmcpMsg := append(append(
		[]byte{CmdIAC, CmdSB, OptGMCP},
		waveEmoji...,
	), CmdIAC, CmdSE)

	events := parser.Receive(gmcpMsg)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Kind != TelnetEventSubnegotiation {
		t.Errorf("Expected Subnegotiation, got %v", events[0].Kind)
	}
	if events[0].Option != OptGMCP {
		t.Errorf("Expected GMCP option, got %d", events[0].Option)
	}
	if !bytes.Equal(events[0].Data, waveEmoji) {
		t.Errorf("Expected wave emoji bytes, got %v", events[0].Data)
	}
}

func TestEscape(t *testing.T) {
	initial := []byte{CmdIAC, CmdSB, 201, CmdIAC, 205, 202, CmdIAC, CmdSE}
	expected := []byte{
		CmdIAC, CmdIAC, CmdSB, 201, CmdIAC, CmdIAC, 205, 202, CmdIAC, CmdIAC, CmdSE,
	}

	escaped := EscapeIAC(initial)
	if !bytes.Equal(escaped, expected) {
		t.Errorf("EscapeIAC failed: expected %v, got %v", expected, escaped)
	}

	unescaped := UnescapeIAC(expected)
	if !bytes.Equal(unescaped, initial) {
		t.Errorf("UnescapeIAC failed: expected %v, got %v", initial, unescaped)
	}
}

func TestUnescape(t *testing.T) {
	initial := []byte{
		CmdIAC, CmdIAC, CmdSB, 201, CmdIAC, CmdIAC, 205, 202, CmdIAC, CmdIAC, CmdSE,
	}
	expected := []byte{CmdIAC, CmdSB, 201, CmdIAC, 205, 202, CmdIAC, CmdSE}

	unescaped := UnescapeIAC(initial)
	if !bytes.Equal(unescaped, expected) {
		t.Errorf("UnescapeIAC failed: expected %v, got %v", expected, unescaped)
	}

	escaped := EscapeIAC(expected)
	if !bytes.Equal(escaped, initial) {
		t.Errorf("EscapeIAC failed: expected %v, got %v", initial, escaped)
	}
}

func TestEscapeRoundtripBugOne(t *testing.T) {
	// The original libtelnet-rs mishandles this input
	data := []byte{CmdIAC, CmdIAC, 228}
	escaped := EscapeIAC(data)
	unescaped := UnescapeIAC(escaped)
	if !bytes.Equal(unescaped, data) {
		t.Errorf("Roundtrip failed: expected %v, got %v", data, unescaped)
	}
}

func TestEscapeRoundtripBugTwo(t *testing.T) {
	// The original libtelnet-rs mishandles this input
	data := []byte{228, CmdIAC, CmdIAC}
	escaped := EscapeIAC(data)
	unescaped := UnescapeIAC(escaped)
	if !bytes.Equal(unescaped, data) {
		t.Errorf("Roundtrip failed: expected %v, got %v", data, unescaped)
	}
}

func TestBadSubnegBuffer(t *testing.T) {
	// Configure opt 0xFF (IAC) as local supported, and local state enabled.
	entry := CompatibilityEntry{Local: true, Remote: false, LocalState: true, RemoteState: false}
	table := FromOptions([][2]byte{{CmdIAC, entry.toU8()}})
	parser := NewParser(table)

	// Receive a malformed subnegotiation - this should not panic
	parser.Receive([]byte{CmdIAC, CmdSB, CmdIAC, CmdSE})
}

func TestCompatibilityTableReset(t *testing.T) {
	table := NewCompatibilityTable()
	entry := CompatibilityEntry{Local: true, Remote: true, LocalState: true, RemoteState: true}
	table.Set(OptGMCP, entry)

	table.ResetStates()
	result := table.Get(OptGMCP)

	if !result.Local || !result.Remote {
		t.Error("ResetStates should preserve support flags")
	}
	if result.LocalState || result.RemoteState {
		t.Error("ResetStates should clear state flags")
	}
}

func TestCompatibilityEntryBitmask(t *testing.T) {
	tests := []struct {
		entry CompatibilityEntry
		want  byte
	}{
		{CompatibilityEntry{Local: true}, bitLocal},
		{CompatibilityEntry{Remote: true}, bitRemote},
		{CompatibilityEntry{LocalState: true}, bitLocalState},
		{CompatibilityEntry{RemoteState: true}, bitRemoteState},
		{CompatibilityEntry{Local: true, Remote: true, LocalState: true, RemoteState: true},
			bitLocal | bitRemote | bitLocalState | bitRemoteState},
	}

	for _, tt := range tests {
		got := tt.entry.toU8()
		if got != tt.want {
			t.Errorf("toU8(%+v) = %d, want %d", tt.entry, got, tt.want)
		}
		roundtrip := entryFromU8(got)
		if roundtrip != tt.entry {
			t.Errorf("entryFromU8(%d) = %+v, want %+v", got, roundtrip, tt.entry)
		}
	}
}

// Tests ported from libmudtelnet compat_tests

func TestParserDiff1(t *testing.T) {
	// options: [(255, 254)]
	// received_data: [[255, 255, 255, 255, 255, 254, 255, 0]]
	table := FromOptions([][2]byte{{255, 254}})
	parser := NewParser(table)
	parser.Receive([]byte{255, 255, 255, 255, 255, 254, 255, 0})
	// Should not panic
}

func TestParserDiff2(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{45, 255, 250, 255})
}

func TestParserDiff3(t *testing.T) {
	table := FromOptions([][2]byte{{0, 1}})
	parser := NewParser(table)
	parser.Receive([]byte{255, 253, 0})
}

func TestParserDiff4(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{255, 250, 255, 255, 240, 250})
}

func TestParserDiff5(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{255, 250, 255, 240, 0})
}

func TestParserDiff6(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{240, 255, 250, 255, 240, 0})
}

func TestParserDiff7(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{255})
}

func TestParserDiff8(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{255, 252, 0})
}

func TestParserDiff9(t *testing.T) {
	parser := NewParserDefault()
	parser.Receive([]byte{254, 255, 255, 255, 254, 0})
}

func TestParserDiff10(t *testing.T) {
	table := FromOptions([][2]byte{{255, 254}, {1, 0}})
	parser := NewParser(table)
	parser.Receive([]byte{255, 253, 255})
}

func TestOutputBuffer(t *testing.T) {
	ob := NewOutputBuffer(TelnetModeUnterminated)

	// Test basic line splitting
	lines := ob.Receive([]byte("line1\r\nline2\nline3"))
	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" || lines[1] != "line2" {
		t.Errorf("Unexpected lines: %v", lines)
	}

	// Prompt should have remaining data
	prompt := ob.Prompt(false)
	if prompt != "line3" {
		t.Errorf("Expected 'line3', got '%s'", prompt)
	}

	// Consume prompt
	prompt = ob.Prompt(true)
	if prompt != "line3" {
		t.Errorf("Expected 'line3', got '%s'", prompt)
	}

	// Should be empty now
	prompt = ob.Prompt(false)
	if prompt != "" {
		t.Errorf("Expected empty prompt, got '%s'", prompt)
	}
}

func TestOutputBufferNewlineVariants(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"CRLF", "a\r\nb\r\n", []string{"a", "b"}},
		{"LF", "a\nb\n", []string{"a", "b"}},
		{"LFCR", "a\n\rb\n\r", []string{"a", "b"}},
		{"Mixed", "a\r\nb\nc\n\r", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ob := NewOutputBuffer(TelnetModeUnterminated)
			lines := ob.Receive([]byte(tt.input))
			if len(lines) != len(tt.expected) {
				t.Errorf("Expected %d lines, got %d: %v", len(tt.expected), len(lines), lines)
				return
			}
			for i := range tt.expected {
				if lines[i] != tt.expected[i] {
					t.Errorf("Line %d: expected '%s', got '%s'", i, tt.expected[i], lines[i])
				}
			}
		})
	}
}

func TestSendText(t *testing.T) {
	ev := SendText("hello")
	if ev.Kind != TelnetEventDataSend {
		t.Errorf("Expected DataSend, got %v", ev.Kind)
	}
	expected := []byte("hello\r\n")
	if !bytes.Equal(ev.Data, expected) {
		t.Errorf("Expected %v, got %v", expected, ev.Data)
	}

	// Test with IAC in text
	ev = SendText(string([]byte{0xFF, 0x41}))
	expected = []byte{0xFF, 0xFF, 0x41, '\r', '\n'}
	if !bytes.Equal(ev.Data, expected) {
		t.Errorf("Expected %v, got %v", expected, ev.Data)
	}
}

func TestNegotiationWILL(t *testing.T) {
	parser := NewParserDefault()
	parser.Options.SupportRemote(OptEcho)

	// Receive WILL ECHO - should respond with DO ECHO
	events := parser.Receive([]byte{CmdIAC, CmdWILL, OptEcho})

	// Should get DataSend (DO) and Negotiation event
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].Kind != TelnetEventDataSend {
		t.Errorf("First event should be DataSend, got %v", events[0].Kind)
	}
	if !bytes.Equal(events[0].Data, []byte{CmdIAC, CmdDO, OptEcho}) {
		t.Errorf("Expected IAC DO ECHO, got %v", events[0].Data)
	}
	if events[1].Kind != TelnetEventNegotiation {
		t.Errorf("Second event should be Negotiation, got %v", events[1].Kind)
	}

	// Check state
	entry := parser.Options.Get(OptEcho)
	if !entry.RemoteState {
		t.Error("RemoteState should be true after WILL")
	}
}

func TestNegotiationWILLUnsupported(t *testing.T) {
	parser := NewParserDefault()
	// OptEcho is NOT supported

	// Receive WILL ECHO - should respond with DONT ECHO
	events := parser.Receive([]byte{CmdIAC, CmdWILL, OptEcho})

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].Kind != TelnetEventDataSend {
		t.Errorf("Expected DataSend, got %v", events[0].Kind)
	}
	if !bytes.Equal(events[0].Data, []byte{CmdIAC, CmdDONT, OptEcho}) {
		t.Errorf("Expected IAC DONT ECHO, got %v", events[0].Data)
	}
}

func TestNegotiationDO(t *testing.T) {
	parser := NewParserDefault()
	parser.Options.SupportLocal(OptNAWS)

	// Receive DO NAWS - should respond with WILL NAWS
	events := parser.Receive([]byte{CmdIAC, CmdDO, OptNAWS})

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d: %+v", len(events), events)
	}
	if !bytes.Equal(events[0].Data, []byte{CmdIAC, CmdWILL, OptNAWS}) {
		t.Errorf("Expected IAC WILL NAWS, got %v", events[0].Data)
	}

	// Check state
	entry := parser.Options.Get(OptNAWS)
	if !entry.LocalState || !entry.RemoteState {
		t.Error("Both LocalState and RemoteState should be true after DO")
	}
}

func TestNegotiationDOUnsupported(t *testing.T) {
	parser := NewParserDefault()
	// OptNAWS is NOT supported

	// Receive DO NAWS - should respond with WONT NAWS
	events := parser.Receive([]byte{CmdIAC, CmdDO, OptNAWS})

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d: %+v", len(events), events)
	}
	if !bytes.Equal(events[0].Data, []byte{CmdIAC, CmdWONT, OptNAWS}) {
		t.Errorf("Expected IAC WONT NAWS, got %v", events[0].Data)
	}
}

func TestLinemodeEnabled(t *testing.T) {
	parser := NewParserDefault()
	parser.Options.SupportRemote(OptLinemode)

	if parser.LinemodeEnabled() {
		t.Error("LinemodeEnabled should be false initially")
	}

	// Enable via WILL
	parser.Receive([]byte{CmdIAC, CmdWILL, OptLinemode})

	if !parser.LinemodeEnabled() {
		t.Error("LinemodeEnabled should be true after WILL")
	}
}

func TestDoubleIACInData(t *testing.T) {
	parser := NewParserDefault()

	// Data with escaped IAC: "Hello\xff\xffWorld"
	events := parser.Receive([]byte{72, 101, 108, 108, 111, 255, 255, 87, 111, 114, 108, 100})

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].Kind != TelnetEventDataReceive {
		t.Errorf("Expected DataReceive, got %v", events[0].Kind)
	}
	// The doubled IAC in data stream should pass through as-is in raw form
	// (unescaping happens in subnegotiation payloads, not raw data)
}

func TestIncompleteIAC(t *testing.T) {
	parser := NewParserDefault()

	// Just IAC alone - should buffer and wait for more
	events := parser.Receive([]byte{CmdIAC})
	if len(events) != 0 {
		t.Errorf("Expected 0 events for lone IAC, got %d", len(events))
	}

	// Complete with GA
	events = parser.Receive([]byte{CmdGA})
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Kind != TelnetEventIAC {
		t.Errorf("Expected IAC event, got %v", events[0].Kind)
	}
	if events[0].Command != CmdGA {
		t.Errorf("Expected GA command, got %d", events[0].Command)
	}
}

func TestNOPCommand(t *testing.T) {
	parser := NewParserDefault()

	events := parser.Receive([]byte{CmdIAC, CmdNOP})
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Kind != TelnetEventIAC {
		t.Errorf("Expected IAC event, got %v", events[0].Kind)
	}
	if events[0].Command != CmdNOP {
		t.Errorf("Expected NOP command, got %d", events[0].Command)
	}
}

func TestSubnegotiationMethod(t *testing.T) {
	parser := NewParserDefault()
	parser.Options.SupportLocal(OptGMCP)

	// Should return nil when option not enabled
	ev := parser.Subnegotiation(OptGMCP, []byte("test"))
	if ev != nil {
		t.Error("Subnegotiation should return nil when option not enabled")
	}

	// Enable the option
	parser.Will(OptGMCP)

	// Now it should work
	ev = parser.Subnegotiation(OptGMCP, []byte("Core.Hello {}"))
	if ev == nil {
		t.Fatal("Subnegotiation should return event when option enabled")
	}
	if ev.Kind != TelnetEventDataSend {
		t.Errorf("Expected DataSend, got %v", ev.Kind)
	}

	// Verify structure: IAC SB OptGMCP [data] IAC SE
	data := ev.Data
	if len(data) < 5 {
		t.Fatalf("Data too short: %v", data)
	}
	if data[0] != CmdIAC || data[1] != CmdSB || data[2] != OptGMCP {
		t.Errorf("Wrong header: %v", data[:3])
	}
	if data[len(data)-2] != CmdIAC || data[len(data)-1] != CmdSE {
		t.Errorf("Wrong footer: %v", data[len(data)-2:])
	}
}

func TestOutputBufferHasNewData(t *testing.T) {
	ob := NewOutputBuffer(TelnetModeUnterminated)

	// Initially no new data
	if ob.HasNewData() {
		t.Error("Expected no new data initially")
	}

	// After receive, should have new data
	ob.Receive([]byte("some text"))
	if !ob.HasNewData() {
		t.Error("Expected new data after Receive")
	}

	// After consuming prompt, should have no new data
	ob.Prompt(true)
	if ob.HasNewData() {
		t.Error("Expected no new data after consuming prompt")
	}

	// Receive more data
	ob.Receive([]byte("more text"))
	if !ob.HasNewData() {
		t.Error("Expected new data after second Receive")
	}

	// Non-consuming Prompt should keep new data flag
	ob.Prompt(false)
	if !ob.HasNewData() {
		t.Error("Expected new data to persist after non-consuming Prompt")
	}
}

func TestOutputBufferInputSent(t *testing.T) {
	// In unterminated mode, InputSent should clear the buffer
	ob := NewOutputBuffer(TelnetModeUnterminated)
	ob.Receive([]byte("prompt> "))

	if ob.Len() == 0 {
		t.Error("Buffer should have data")
	}

	ob.InputSent()

	if ob.Len() != 0 {
		t.Error("Buffer should be empty after InputSent in unterminated mode")
	}
	if ob.HasNewData() {
		t.Error("Should have no new data after InputSent")
	}

	// In terminated mode, InputSent should NOT clear the buffer
	ob2 := NewOutputBuffer(TelnetModeTerminatedPrompt)
	ob2.Receive([]byte("prompt> "))

	if ob2.Len() == 0 {
		t.Error("Buffer should have data")
	}

	ob2.InputSent()

	if ob2.Len() == 0 {
		t.Error("Buffer should NOT be cleared in terminated mode")
	}
}

func TestOutputBufferClearResetsNewData(t *testing.T) {
	ob := NewOutputBuffer(TelnetModeTerminatedPrompt)
	ob.Receive([]byte("data"))

	if !ob.HasNewData() {
		t.Error("Should have new data")
	}

	ob.Clear()

	if ob.HasNewData() {
		t.Error("Clear should reset new data flag")
	}
	if ob.Len() != 0 {
		t.Error("Clear should empty buffer")
	}
}
