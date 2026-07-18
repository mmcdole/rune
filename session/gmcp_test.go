package session

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/network"
)

// GMCP handler routing and window-size NAWS reporting are covered
// end-to-end by test/e2e/scenarios/gmcp.json and window-size.json; this
// file keeps the assertions on exact handshake payloads.

// TestGMCPEnabledTriggersHandshake verifies the enabled notification
// reaches Lua and the Core.Hello handshake goes back out through the
// network layer.
func TestGMCPEnabledTriggersHandshake(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("subscribe", `rune.gmcp.subscribe("Char")`); err != nil {
		t.Fatal(err)
	}

	// The network layer flips gmcpActive before emitting the enabled
	// notification; mirror that ordering.
	net.gmcpActive = true
	s.handleNetworkOutput(network.Output{Kind: network.OutputGMCPEnabled})

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
