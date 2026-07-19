package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/rune/network"
)

// TestGMCPStateSurvivesReloadMidConnection reproduces the /reload
// regression: negotiation is a connection-lifetime fact, but it used
// to be cached in the VM, so a reload mid-connection left the fresh
// Lua state believing GMCP was down - is_enabled() lied and
// subscription changes in the edited init.lua never reached the
// server until reconnect. State must instead be queried from the
// connection, making reload transparent.
func TestGMCPStateSurvivesReloadMidConnection(t *testing.T) {
	dir := t.TempDir()
	writeInit := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "init.lua"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeInit(`rune.gmcp.subscribe("Char")`)
	s, net, uiMock := bootSessionInDir(t, dir)
	uiMock.drainPrinted()

	// Server negotiates GMCP on the live connection.
	net.connected = true
	net.gmcpActive = true
	s.handleNetworkOutput(network.Output{Kind: network.OutputGMCPEnabled})

	sent := net.drainGMCPSent()
	if len(sent) < 2 || sent[0].Package != "Core.Hello" {
		t.Fatalf("handshake must send Core.Hello then supports, got %v", sent)
	}
	if last := sent[len(sent)-1]; last.Package != "Core.Supports.Set" || last.Data != `["Char 1"]` {
		t.Fatalf("initial supports = %v", sent)
	}

	// User edits init.lua (adds a package) and reloads mid-connection.
	writeInit(strings.Join([]string{
		`rune.gmcp.subscribe("Char")`,
		`rune.gmcp.subscribe("Room")`,
		`rune.echo("RELOAD-GMCP-UP=" .. tostring(rune.gmcp.is_enabled()))`,
	}, "\n"))
	s.Reload()
	cb := <-s.asyncResults // reload is deferred
	cb()

	// is_enabled() must stay truthful in the reloaded VM.
	if printed := uiMock.drainPrinted(); !contains(printed, "RELOAD-GMCP-UP=true") {
		t.Errorf("is_enabled() false after reload on a GMCP-active connection; printed: %v", printed)
	}

	// The subscription change must reach the server immediately - and
	// Core.Hello must NOT be re-sent (once per connection, per spec).
	sent = net.drainGMCPSent()
	var lastSupports string
	for _, msg := range sent {
		if msg.Package == "Core.Hello" {
			t.Errorf("Core.Hello re-sent on reload: %v", sent)
		}
		if msg.Package == "Core.Supports.Set" {
			lastSupports = msg.Data
		}
	}
	if lastSupports != `["Char 1","Room 1"]` {
		t.Errorf("subscription change did not reach the server after reload; last supports = %q, sends = %v",
			lastSupports, sent)
	}
}
