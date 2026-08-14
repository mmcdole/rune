package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGMCPStateSurvivesReloadMidConnection verifies that connection-scoped
// protocol state remains authoritative while /reload replaces the Lua VM.
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
	serverNegotiatesGMCP(s)

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
	awaitInternalEvent(t, s)

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
