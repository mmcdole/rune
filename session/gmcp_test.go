package session

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/ui"
)

// TestGMCPMessageRoutedToLuaHandler drives a GMCP message through the
// same path the event loop uses (network output -> session event ->
// engine dispatch) and verifies a Lua handler sees the decoded data.
func TestGMCPMessageRoutedToLuaHandler(t *testing.T) {
	s, _, uiMock := newTestSession(t)

	if err := s.engine.DoString("setup", `
		rune.gmcp.on("Char.Vitals", function(data)
			rune.echo("HP is " .. data.hp)
		end)
	`); err != nil {
		t.Fatal(err)
	}

	s.handleNetworkOutput(network.Output{
		Kind:    network.OutputGMCP,
		Package: "Char.Vitals",
		Payload: `{"hp":42}`,
	})

	if printed := uiMock.drainPrinted(); !contains(printed, "HP is 42") {
		t.Errorf("expected handler output, got %v", printed)
	}
}

// TestGMCPEnabledTriggersHandshake verifies the enabled notification
// reaches Lua and the Core.Hello handshake goes back out through the
// network layer.
func TestGMCPEnabledTriggersHandshake(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("subscribe", `rune.gmcp.subscribe("Char")`); err != nil {
		t.Fatal(err)
	}

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

// TestWindowSizeForwardedToNetwork verifies resizes reach the network
// layer, where NAWS reports them to the server.
func TestWindowSizeForwardedToNetwork(t *testing.T) {
	s, net, _ := newTestSession(t)

	s.handleUIMessage(ui.WindowSizeChangedMsg{Width: 132, Height: 43})

	net.mu.Lock()
	w, h := net.windowW, net.windowH
	net.mu.Unlock()
	if w != 132 || h != 43 {
		t.Errorf("network size = %dx%d, want 132x43", w, h)
	}
}
