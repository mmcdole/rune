package lua

// The hooks Rune's own core installs: every one is named, the public ones
// (echo-style, log-output, log-echo, gmcp-hello, notice-<event>) can be
// replaced by re-registering the name, and splitting jobs apart keeps a
// replacement from breaking something unrelated.

import (
	"strings"
	"testing"
)

func TestEveryCoreEntryIsNamed(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		local namespaces = { hooks = rune.hooks, binds = rune.binds, bars = rune.bars,
			timer = rune.timer, alias = rune.alias, trigger = rune.trigger,
			command = rune.command, gmcp = rune.gmcp }
		for label, ns in pairs(namespaces) do
			for _, row in ipairs(ns.list()) do
				assert(row.name, label .. ": unnamed core entry @" .. tostring(row.source))
			end
		end
	`)
}

func TestEchoStyleIsReplacedByName(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	if styled, _ := engine.OnEcho("look"); !strings.Contains(styled, "> look") {
		t.Fatalf("default echo = %q", styled)
	}
	assertLua(t, engine, `
		rune.hooks.on("echo", function(text) return "» " .. text end, { name = "echo-style" })
	`)
	if styled, _ := engine.OnEcho("look"); styled != "» look" {
		t.Fatalf("replaced echo = %q", styled)
	}
}

// Replacing the hello used to drop Core.Supports.Set, because both were
// sent by the one gmcp-hello hook.
func TestGMCPHelloReplacementKeepsSubscriptions(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		rune.gmcp.subscribe("Char")
		rune.hooks.on("gmcp_enabled", function()
			rune.gmcp.send("Core.Hello", { client = "Custom", version = "1" })
		end, { name = "gmcp-hello" })
	`)
	host.GMCPNegotiated = true
	engine.NotifyGMCPEnabled()
	if len(host.GMCPSends) != 2 {
		t.Fatalf("sends = %+v, want hello then supports", host.GMCPSends)
	}
	if host.GMCPSends[0].Package != "Core.Hello" || !strings.Contains(host.GMCPSends[0].Data, "Custom") {
		t.Errorf("first send = %+v, want the custom hello", host.GMCPSends[0])
	}
	if host.GMCPSends[1].Package != "Core.Supports.Set" || host.GMCPSends[1].Data != `["Char 1"]` {
		t.Errorf("second send = %+v, want the subscriptions", host.GMCPSends[1])
	}
}

func TestNoticesAreNamedPerEvent(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `rune.hooks.disable("notice-reloaded")`)
	host.DrainPrintCalls()
	engine.NotifyReloaded()
	engine.NotifyLoaded("x.lua")
	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if strings.Contains(printed, "Scripts reloaded") {
		t.Error("disabled notice still printed")
	}
	if !strings.Contains(printed, "Loaded: x.lua") {
		t.Error("an unrelated notice stopped printing")
	}
}

// Remembering the last address used to live inside the connected notice,
// so silencing the notice broke /reconnect.
func TestSilencedConnectNoticeStillRemembersAddress(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `rune.hooks.disable("notice-connected")`)
	engine.NotifyConnected("mud.example.com:4000")
	assertLua(t, engine, `
		assert(rune.store.get("last_address") == "mud.example.com:4000",
			"last_address = " .. tostring(rune.store.get("last_address")))
	`)
}
