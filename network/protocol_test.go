package network

import (
	"bytes"
	"fmt"
	"testing"
)

type readPartition struct {
	name   string
	chunks [][]byte
}

func readPartitions(wire []byte) []readPartition {
	partitions := []readPartition{{name: "unsplit", chunks: [][]byte{wire}}}
	for cut := 1; cut < len(wire); cut++ {
		partitions = append(partitions, readPartition{
			name:   fmt.Sprintf("cut_%d", cut),
			chunks: [][]byte{wire[:cut], wire[cut:]},
		})
	}
	bytewise := make([][]byte, len(wire))
	for i := range wire {
		bytewise[i] = wire[i : i+1]
	}
	return append(partitions, readPartition{name: "bytewise", chunks: bytewise})
}

func TestGMCPFrameRequiresNegotiation(t *testing.T) {
	protocol := NewProtocol(80, 24)
	if _, err := protocol.GMCPFrame("Core.Hello", "{}"); err == nil {
		t.Fatal("GMCPFrame succeeded before negotiation")
	}
}

func TestProtocolEmitsGMCPEnabledOncePerActivation(t *testing.T) {
	p := NewProtocol(80, 24)
	batch := EventBatch{Events: []TelnetEvent{
		{Kind: TelnetEventNegotiation, Command: CmdWILL, Option: OptGMCP},
		{Kind: TelnetEventNegotiation, Command: CmdDO, Option: OptGMCP},
		{Kind: TelnetEventNegotiation, Command: CmdWONT, Option: OptGMCP},
		{Kind: TelnetEventNegotiation, Command: CmdDONT, Option: OptGMCP},
		{Kind: TelnetEventNegotiation, Command: CmdWILL, Option: OptGMCP},
	}}

	enabled := 0
	p.Process(batch, func(effect Effect) bool {
		if effect.Kind == EffectGMCPEnabled {
			enabled++
		}
		return true
	})
	if enabled != 2 {
		t.Fatalf("enabled effects = %d, want one per activation epoch", enabled)
	}
}

func TestProtocolTracksGMCPDirectionsIndependently(t *testing.T) {
	p := NewProtocol(80, 24)
	events := []TelnetEvent{
		{Kind: TelnetEventNegotiation, Command: CmdWILL, Option: OptGMCP},
		{Kind: TelnetEventNegotiation, Command: CmdDO, Option: OptGMCP},
		{Kind: TelnetEventNegotiation, Command: CmdWONT, Option: OptGMCP},
	}
	enabled := 0
	p.Process(EventBatch{Events: events}, func(effect Effect) bool {
		if effect.Kind == EffectGMCPEnabled {
			enabled++
		}
		return true
	})
	if enabled != 1 {
		t.Fatalf("enabled effects = %d, want one false-to-true transition", enabled)
	}
	if !p.GMCPActive() {
		t.Fatal("WONT disabled GMCP while the local DO direction remained active")
	}

	p.Process(EventBatch{Events: []TelnetEvent{
		{Kind: TelnetEventNegotiation, Command: CmdDONT, Option: OptGMCP},
	}}, func(Effect) bool { return true })
	if p.GMCPActive() {
		t.Fatal("GMCP remained active after both directions were disabled")
	}
}

func TestProtocolLocalEchoFollowsOnlyRemoteEchoDirection(t *testing.T) {
	p := NewProtocol(80, 24)
	emit := func(Effect) bool { return true }

	p.Process(EventBatch{Events: []TelnetEvent{
		{Kind: TelnetEventNegotiation, Command: CmdWILL, Option: OptEcho},
	}}, emit)
	if p.LocalEchoEnabled() {
		t.Fatal("local echo remained enabled after server WILL ECHO")
	}

	// DO/DONT concern our side of the option. They cannot change whether the
	// remote server is echoing our input.
	p.Process(EventBatch{Events: []TelnetEvent{
		{Kind: TelnetEventNegotiation, Command: CmdDO, Option: OptEcho},
		{Kind: TelnetEventNegotiation, Command: CmdDONT, Option: OptEcho},
	}}, emit)
	if p.LocalEchoEnabled() {
		t.Fatal("local echo followed the orthogonal local ECHO direction")
	}

	p.Process(EventBatch{Events: []TelnetEvent{
		{Kind: TelnetEventNegotiation, Command: CmdWONT, Option: OptEcho},
	}}, emit)
	if !p.LocalEchoEnabled() {
		t.Fatal("local echo remained disabled after server WONT ECHO")
	}
}

func TestProtocolUsesBatchSecurityForIdentityReplies(t *testing.T) {
	p := NewProtocol(80, 24)
	request := TelnetEvent{Kind: TelnetEventSubnegotiation, Option: OptTTYPE, Data: []byte{CmdSEND}}
	var frames [][]byte
	p.Process(EventBatch{Secure: true, Events: []TelnetEvent{request, request, request}}, func(effect Effect) bool {
		if effect.Kind == EffectSendFrame {
			frames = append(frames, effect.Data)
		}
		return true
	})
	if len(frames) != 3 {
		t.Fatalf("identity replies = %d, want three", len(frames))
	}
	want := []byte("MTTS 2831") // Rune's advertised capabilities, including MTTS TLS.
	if !bytes.Contains(frames[2], want) {
		t.Fatalf("TLS MTTS reply = %q, want TLS capability bit", frames[2])
	}
}

func TestProtocolStopsWhenEmitterRejectsAnEffect(t *testing.T) {
	p := NewProtocol(80, 24)
	batch := EventBatch{Events: []TelnetEvent{
		{Kind: TelnetEventDataReceive, Data: []byte("first")},
		{Kind: TelnetEventIAC, Command: CmdGA},
		{Kind: TelnetEventDataReceive, Data: []byte("second")},
	}}

	var got []Effect
	ok := p.Process(batch, func(effect Effect) bool {
		got = append(got, effect)
		return false
	})
	if ok {
		t.Fatal("Process reported success after the emitter rejected an effect")
	}
	if len(got) != 1 || got[0].Kind != EffectServerData || string(got[0].Data) != "first" {
		t.Fatalf("effects before stop = %+v, want only the first server-data effect", got)
	}
}

func TestProtocolRecordMarksDoNotDependOnReadSplits(t *testing.T) {
	tests := []struct {
		name    string
		command byte
		effect  EffectKind
	}{
		{name: "GA", command: CmdGA, effect: EffectGA},
		{name: "EOR", command: CmdEOR, effect: EffectEOR},
	}

	for _, tt := range tests {
		wire := append([]byte("Username:"), CmdIAC, tt.command)
		for _, partition := range readPartitions(wire) {
			t.Run(tt.name+"/"+partition.name, func(t *testing.T) {
				parser := newParser(newCompatibilityTable())
				protocol := NewProtocol(80, 24)
				var text bytes.Buffer
				boundaries := 0

				for _, chunk := range partition.chunks {
					ok := protocol.Process(EventBatch{Events: parser.Receive(chunk)}, func(effect Effect) bool {
						switch effect.Kind {
						case EffectServerData:
							if boundaries != 0 {
								t.Fatalf("server data %q arrived after prompt boundary", effect.Data)
							}
							text.Write(effect.Data)
						case tt.effect:
							boundaries++
							if got := text.String(); got != "Username:" {
								t.Fatalf("text before mark = %q, want Username:", got)
							}
						case EffectGA, EffectEOR:
							t.Fatalf("wrong prompt-boundary effect %v", effect.Kind)
						default:
							t.Fatalf("unexpected effect %+v", effect)
						}
						return true
					})
					if !ok {
						t.Fatal("Process stopped")
					}
				}

				if got := text.String(); got != "Username:" {
					t.Fatalf("server text = %q, want Username:", got)
				}
				if boundaries != 1 {
					t.Fatalf("prompt boundaries = %d, want one", boundaries)
				}
			})
		}
	}
}

func TestProtocolGMCPWireOrderDoesNotDependOnReadSplits(t *testing.T) {
	wire := append([]byte{CmdIAC, CmdWILL, OptGMCP},
		subnegFrame(OptGMCP, []byte(`Char.Vitals {"hp":100}`))...)
	wire = append(wire, CmdIAC, CmdWONT, OptGMCP)

	want := []Effect{
		{Kind: EffectSendFrame, Data: []byte{CmdIAC, CmdDO, OptGMCP}},
		{Kind: EffectGMCPEnabled},
		{Kind: EffectSendFrame, Data: subnegFrame(OptGMCP, []byte(`Core.Hello {}`))},
		{Kind: EffectGMCPMessage, Package: "Char.Vitals", Payload: `{"hp":100}`},
		{Kind: EffectSendFrame, Data: subnegFrame(OptGMCP, []byte(`Char.Response {}`))},
		{Kind: EffectSendFrame, Data: []byte{CmdIAC, CmdDONT, OptGMCP}},
	}
	run := func(t *testing.T, chunks ...[]byte) {
		t.Helper()
		parser := NewParser()
		protocol := NewProtocol(80, 24)
		var got []Effect
		for _, chunk := range chunks {
			batch := EventBatch{Events: parser.Receive(chunk)}
			if !protocol.Process(batch, func(effect Effect) bool {
				got = append(got, effect)
				if effect.Kind == EffectGMCPEnabled {
					frame, err := protocol.GMCPFrame("Core.Hello", "{}")
					if err != nil {
						t.Fatal(err)
					}
					if len(frame) == 0 {
						t.Fatal("empty GMCP frame")
					}
					got = append(got, Effect{Kind: EffectSendFrame, Data: frame})
				}
				if effect.Kind == EffectGMCPMessage {
					frame, err := protocol.GMCPFrame("Char.Response", "{}")
					if err != nil {
						t.Fatal(err)
					}
					got = append(got, Effect{Kind: EffectSendFrame, Data: frame})
				}
				return true
			}) {
				t.Fatal("Process stopped")
			}
		}
		if len(got) != len(want) {
			t.Fatalf("effects = %+v, want %+v", got, want)
		}
		for i := range want {
			if got[i].Kind != want[i].Kind || got[i].Package != want[i].Package || got[i].Payload != want[i].Payload || !bytes.Equal(got[i].Data, want[i].Data) {
				t.Fatalf("effect %d = %+v, want %+v", i, got[i], want[i])
			}
		}
		if protocol.GMCPActive() {
			t.Fatal("GMCP remained active after WONT")
		}
	}

	for _, partition := range readPartitions(wire) {
		t.Run(partition.name, func(t *testing.T) {
			run(t, partition.chunks...)
		})
	}
}
