package session

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mmcdole/rune/network"
)

// GMCP handler routing and window-size NAWS reporting are covered end-to-end
// by scenarios. These tests pin Session's handshake and batch-order contract.

// TestGMCPEnabledTriggersHandshake verifies the enabled notification
// reaches Lua and the Core.Hello handshake goes back out through the
// network layer.
func TestGMCPEnabledTriggersHandshake(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("subscribe", `rune.gmcp.subscribe("Char")`); err != nil {
		t.Fatal(err)
	}

	serverNegotiatesGMCP(s)

	sent := net.drainGMCPSent()
	if len(sent) != 2 {
		t.Fatalf("expected Core.Hello + Core.Supports.Set, got %v", sent)
	}
	if sent[0].Package != "Core.Hello" || !strings.Contains(sent[0].Data, `"client":"Rune"`) {
		t.Errorf("Core.Hello = %+v", sent[0])
	}
	if sent[1].Package != "Core.Supports.Set" || sent[1].Data != `["Char 1"]` {
		t.Errorf("Core.Supports.Set = %+v", sent[1])
	}

	if err := s.engine.DoString("check", `assert(rune.gmcp.is_enabled())`); err != nil {
		t.Error(err)
	}
}

func TestGMCPBatchEffectsStayInWireOrderAcrossLuaCallbacks(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("ordered GMCP callbacks", `
		rune.gmcp.subscribe("Char")
		rune.gmcp.on("Char.Vitals", function()
			rune.gmcp.send("Core.Ping", { sequence = 1 })
		end)
	`); err != nil {
		t.Fatal(err)
	}

	doGMCP := []byte{network.CmdIAC, network.CmdDO, network.OptGMCP}
	dontGMCP := []byte{network.CmdIAC, network.CmdDONT, network.OptGMCP}
	serverBatch(s,
		network.TelnetEvent{Kind: network.TelnetEventDataSend, Data: doGMCP},
		network.TelnetEvent{Kind: network.TelnetEventNegotiation, Command: network.CmdWILL, Option: network.OptGMCP},
		network.TelnetEvent{Kind: network.TelnetEventSubnegotiation, Option: network.OptGMCP, Data: []byte(`Char.Vitals {"hp":10}`)},
		network.TelnetEvent{Kind: network.TelnetEventDataSend, Data: dontGMCP},
		network.TelnetEvent{Kind: network.TelnetEventNegotiation, Command: network.CmdWONT, Option: network.OptGMCP},
	)

	if s.GMCPActive() {
		t.Fatal("GMCP remained active after trailing WONT")
	}
	frames := net.drainFrames()
	if len(frames) != 5 {
		t.Fatalf("frames = %d, want DO, Hello, Supports, handler send, DONT: % x", len(frames), frames)
	}
	if !bytes.Equal(frames[0], doGMCP) || !bytes.Equal(frames[4], dontGMCP) {
		t.Fatalf("negotiation frames out of order: % x", frames)
	}
	gmcp := net.drainGMCPSent()
	if len(gmcp) != 3 || gmcp[0].Package != "Core.Hello" || gmcp[1].Package != "Core.Supports.Set" || gmcp[2].Package != "Core.Ping" {
		t.Fatalf("GMCP callback sends out of order: %+v", gmcp)
	}
}
