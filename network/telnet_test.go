package network

import (
	"bytes"
	"testing"
)

// TestParserHandlesSplitDoNegotiation verifies that an incomplete command is
// retained until its option byte arrives in a later parser call.
func TestParserHandlesSplitDoNegotiation(t *testing.T) {
	parser := NewParser()

	// First chunk ends mid-command: IAC DO (missing option) - should emit nothing.
	events := parser.Receive([]byte{CmdIAC, CmdDO})
	if len(events) != 0 {
		t.Fatalf("expected no events yet, got %v", events)
	}

	// Second chunk provides the option byte; we should reply WILL SGA.
	events = parser.Receive([]byte{OptSGA})
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
	expected := []byte{CmdIAC, CmdWILL, OptSGA}
	if !bytes.Equal(reply, expected) {
		t.Fatalf("unexpected reply: want %v got %v", expected, reply)
	}
}

func TestDefaultCompatibilityNegotiationPolicy(t *testing.T) {
	groups := []struct {
		name       string
		command    byte
		options    []byte
		reply      byte
		eventCount int
	}{
		{
			name:       "accept server options",
			command:    CmdWILL,
			options:    []byte{OptMCCP2, OptGMCP, OptEcho, OptSGA, OptEOR},
			reply:      CmdDO,
			eventCount: 2,
		},
		{
			name:       "accept client options",
			command:    CmdDO,
			options:    []byte{OptTTYPE, OptNAWS, OptCharset, OptNewEnviron},
			reply:      CmdWILL,
			eventCount: 2,
		},
		{
			name:       "refuse unimplemented server options",
			command:    CmdWILL,
			options:    []byte{OptMCCP3, OptMSSP, OptZMP, OptLinemode},
			reply:      CmdDONT,
			eventCount: 1,
		},
		{
			name:       "refuse unimplemented client options",
			command:    CmdDO,
			options:    []byte{OptMCCP3, OptMSSP, OptZMP, OptLinemode},
			reply:      CmdWONT,
			eventCount: 1,
		},
		{
			name:       "refuse unsupported direction",
			command:    CmdDO,
			options:    []byte{OptEcho, OptEOR},
			reply:      CmdWONT,
			eventCount: 1,
		},
	}

	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			for _, option := range group.options {
				parser := NewParser()
				events := parser.Receive([]byte{CmdIAC, group.command, option})
				if len(events) != group.eventCount {
					t.Fatalf("option %d produced %d events, want %d: %+v", option, len(events), group.eventCount, events)
				}
				want := []byte{CmdIAC, group.reply, option}
				assertReply(t, events, want, group.name, option)
			}
		})
	}
}

func assertReply(t *testing.T, events []TelnetEvent, want []byte, cmd string, opt byte) {
	t.Helper()
	for _, ev := range events {
		if ev.Kind == TelnetEventDataSend {
			if !bytes.Equal(ev.Data, want) {
				t.Errorf("%s %d: want reply %v, got %v", cmd, opt, want, ev.Data)
			}
			return
		}
	}
	t.Errorf("%s %d: expected a negotiation reply, got none", cmd, opt)
}

func TestParserStopsAtMCCPActivation(t *testing.T) {
	table := newCompatibilityTable()
	table.set(OptMCCP2, compatibilityEntry{Remote: true, RemoteState: true})
	parser := newParser(table)
	remainder := []byte("compressed bytes")
	wire := append(subnegFrame(OptMCCP2, nil), remainder...)

	events := parser.Receive(wire)
	if len(events) != 2 {
		t.Fatalf("events = %+v, want activation and compressed remainder", events)
	}
	if event := events[0]; event.Kind != TelnetEventSubnegotiation || event.Option != OptMCCP2 {
		t.Fatalf("activation event = %+v", event)
	}
	if event := events[1]; event.Kind != TelnetEventDecompressImmediate || !bytes.Equal(event.Data, remainder) {
		t.Fatalf("remainder event = %+v, want %q", event, remainder)
	}
}

// An unsolicited compression subnegotiation must not switch the read path or
// swallow the plaintext that follows it: MCCP3 (client->server compression)
// stays refused outright, and MCCP2 splits the stream only once negotiated.
func TestUnsolicitedMCCPSubnegotiationDoesNotSwallowFollowingText(t *testing.T) {
	for _, opt := range []byte{OptMCCP2, OptMCCP3} {
		parser := NewParser()
		wire := append(subnegFrame(opt, nil), []byte("plain text")...)

		events := parser.Receive(wire)
		if len(events) != 1 || events[0].Kind != TelnetEventDataReceive ||
			string(events[0].Data) != "plain text" {
			t.Fatalf("option %d: events = %+v, want only the plaintext data", opt, events)
		}
	}
}

func TestSubnegSeparateReceives(t *testing.T) {
	table := newCompatibilityTable()
	table.set(OptGMCP, compatibilityEntry{Local: true, LocalState: true})
	parser := newParser(table)

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

func TestSubnegotiationTreatsBareSEAsPayload(t *testing.T) {
	table := newCompatibilityTable()
	table.set(OptGMCP, compatibilityEntry{Local: true, LocalState: true})
	parser := newParser(table)

	// A bare SE byte is data; only the two-byte IAC SE sequence ends the frame.
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

func TestIACEscaping(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		escaped []byte
	}{
		{
			name: "telnet-shaped data",
			data: []byte{CmdIAC, CmdSB, OptGMCP, CmdIAC, 205, 202, CmdIAC, CmdSE},
			escaped: []byte{
				CmdIAC, CmdIAC, CmdSB, OptGMCP, CmdIAC, CmdIAC, 205, 202, CmdIAC, CmdIAC, CmdSE,
			},
		},
		{
			name:    "adjacent IAC before data",
			data:    []byte{CmdIAC, CmdIAC, 228},
			escaped: []byte{CmdIAC, CmdIAC, CmdIAC, CmdIAC, 228},
		},
		{
			name:    "adjacent IAC after data",
			data:    []byte{228, CmdIAC, CmdIAC},
			escaped: []byte{228, CmdIAC, CmdIAC, CmdIAC, CmdIAC},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := escapeIAC(tt.data)
			if !bytes.Equal(escaped, tt.escaped) {
				t.Fatalf("escapeIAC(%v) = %v, want %v", tt.data, escaped, tt.escaped)
			}
			if got := unescapeIAC(escaped); !bytes.Equal(got, tt.data) {
				t.Fatalf("unescapeIAC(escapeIAC(%v)) = %v", tt.data, got)
			}
		})
	}
}

func TestCompatibilityEntryBitmask(t *testing.T) {
	tests := []struct {
		entry compatibilityEntry
		want  byte
	}{
		{compatibilityEntry{Local: true}, bitLocal},
		{compatibilityEntry{Remote: true}, bitRemote},
		{compatibilityEntry{LocalState: true}, bitLocalState},
		{compatibilityEntry{RemoteState: true}, bitRemoteState},
		{compatibilityEntry{Local: true, Remote: true, LocalState: true, RemoteState: true},
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

func TestParserHandlesMalformedStreams(t *testing.T) {
	// Each case is a valid incremental parser state even when the byte stream is
	// incomplete or nonsensical. The contract is that parsing remains safe.
	tests := []struct {
		name    string
		entries map[byte]compatibilityEntry
		wire    []byte
	}{
		{
			name: "IAC option in unfinished subnegotiation",
			entries: map[byte]compatibilityEntry{
				CmdIAC: {Local: true, LocalState: true},
			},
			wire: []byte{CmdIAC, CmdSB, CmdIAC, CmdSE},
		},
		{
			name: "repeated escaped IAC before truncated command",
			entries: map[byte]compatibilityEntry{
				CmdIAC: {Remote: true, LocalState: true, RemoteState: true},
			},
			wire: []byte{255, 255, 255, 255, 255, 254, 255, 0},
		},
		{name: "data before unfinished subnegotiation", wire: []byte{45, 255, 250, 255}},
		{
			name:    "DO supported NUL option",
			entries: map[byte]compatibilityEntry{0: {Local: true}},
			wire:    []byte{255, 253, 0},
		},
		{name: "escaped IAC before bare SE", wire: []byte{255, 250, 255, 255, 240, 250}},
		{name: "SE after IAC option with trailing data", wire: []byte{255, 250, 255, 240, 0}},
		{name: "bare SE before malformed subnegotiation", wire: []byte{240, 255, 250, 255, 240, 0}},
		{name: "lone IAC", wire: []byte{255}},
		{name: "WONT NUL option", wire: []byte{255, 252, 0}},
		{name: "data and escaped IAC before DONT", wire: []byte{254, 255, 255, 255, 254, 0}},
		{
			name: "DO IAC option",
			entries: map[byte]compatibilityEntry{
				CmdIAC: {Remote: true, LocalState: true, RemoteState: true},
			},
			wire: []byte{255, 253, 255},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := newCompatibilityTable()
			for option, entry := range tt.entries {
				table.set(option, entry)
			}
			newParser(table).Receive(tt.wire)
		})
	}
}

func TestNegotiationWILL(t *testing.T) {
	parser := newParser(newCompatibilityTable())
	parser.options.supportRemote(OptEcho)

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
	entry := parser.options.get(OptEcho)
	if !entry.RemoteState {
		t.Error("RemoteState should be true after WILL")
	}
}

func TestNegotiationDO(t *testing.T) {
	parser := newParser(newCompatibilityTable())
	parser.options.supportLocal(OptNAWS)

	// Receive DO NAWS - should respond with WILL NAWS
	events := parser.Receive([]byte{CmdIAC, CmdDO, OptNAWS})

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d: %+v", len(events), events)
	}
	if !bytes.Equal(events[0].Data, []byte{CmdIAC, CmdWILL, OptNAWS}) {
		t.Errorf("Expected IAC WILL NAWS, got %v", events[0].Data)
	}

	// Check state
	entry := parser.options.get(OptNAWS)
	if !entry.LocalState {
		t.Error("LocalState should be true after accepted DO")
	}
	if entry.RemoteState {
		t.Error("RemoteState must stay false: the server never sent WILL")
	}
}

func TestDoubleIACInData(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		want []byte
	}{
		{
			name: "middle of data",
			wire: append(append([]byte("Hello"), CmdIAC, CmdIAC), []byte("World")...),
			want: append(append([]byte("Hello"), CmdIAC), []byte("World")...),
		},
		{
			name: "three byte input is not negotiation",
			wire: []byte{CmdIAC, CmdIAC, OptEcho},
			want: []byte{CmdIAC, OptEcho},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := newParser(newCompatibilityTable())
			events := parser.Receive(tt.wire)
			if len(events) != 1 {
				t.Fatalf("events = %+v, want one data event", events)
			}
			if events[0].Kind != TelnetEventDataReceive {
				t.Fatalf("event kind = %v, want DataReceive", events[0].Kind)
			}
			if !bytes.Equal(events[0].Data, tt.want) {
				t.Errorf("data = %v, want %v", events[0].Data, tt.want)
			}
		})
	}
}

func TestDoubleIACSplitAcrossReceives(t *testing.T) {
	parser := newParser(newCompatibilityTable())

	events := parser.Receive(append([]byte("Hello"), CmdIAC))
	if len(events) != 1 || events[0].Kind != TelnetEventDataReceive || string(events[0].Data) != "Hello" {
		t.Fatalf("first receive events = %+v, want data Hello", events)
	}

	events = parser.Receive(append([]byte{CmdIAC}, []byte("World")...))
	want := append([]byte{CmdIAC}, []byte("World")...)
	if len(events) != 1 {
		t.Fatalf("second receive events = %+v, want one data event", events)
	}
	if events[0].Kind != TelnetEventDataReceive {
		t.Fatalf("event kind = %v, want DataReceive", events[0].Kind)
	}
	if !bytes.Equal(events[0].Data, want) {
		t.Errorf("data = %v, want %v", events[0].Data, want)
	}
}

func TestIncompleteIAC(t *testing.T) {
	parser := newParser(newCompatibilityTable())

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
	parser := newParser(newCompatibilityTable())

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
