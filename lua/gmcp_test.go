package lua

// GMCP policy tests (70_gmcp.lua): dispatch, decoding, handshake,
// quarantine, and the send bridge, driven against MockHost. The e2e
// wiring proof lives in test/e2e/scenarios/gmcp.json.

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/version"
)

// TestGMCPDispatchDecodesAndMatchesCaseInsensitively verifies GMCP
// handlers receive the decoded JSON value and that package matching
// is case-insensitive per the spec.
func TestGMCPDispatchDecodesAndMatchesCaseInsensitively(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		gmcp_seen = {}
		rune.gmcp.on("Char.Vitals", function(data, package)
			table.insert(gmcp_seen, { data = data, package = package })
		end)
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatal(err)
	}

	engine.OnGMCP("Char.Vitals", `{"hp":100,"maxhp":200,"buffs":["haste","shield"]}`)
	engine.OnGMCP("char.vitals", `{"hp":50}`) // different case, same handler
	engine.OnGMCP("Room.Info", `{"num":1}`)   // no handler: must not error

	check := `
		assert(#gmcp_seen == 2, "expected 2 dispatches, got " .. #gmcp_seen)
		assert(gmcp_seen[1].data.hp == 100)
		assert(gmcp_seen[1].data.maxhp == 200)
		assert(gmcp_seen[1].data.buffs[2] == "shield")
		assert(gmcp_seen[1].package == "Char.Vitals")
		assert(gmcp_seen[2].data.hp == 50)
		assert(gmcp_seen[2].package == "char.vitals")
	`
	if err := engine.DoString("check", check); err != nil {
		t.Fatalf("GMCP dispatch check failed: %v", err)
	}
}

// TestGMCPCatchAllHook verifies the generic "gmcp" hook sees every
// message, including ones with no body (data == nil).
func TestGMCPCatchAllHook(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		gmcp_all = {}
		rune.hooks.on("gmcp", function(package, data, raw)
			table.insert(gmcp_all, { package = package, data = data, raw = raw })
		end)
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatal(err)
	}

	engine.OnGMCP("Core.Ping", "")
	engine.OnGMCP("Comm.Channel", `{"chan":"gossip"}`)

	check := `
		assert(#gmcp_all == 2)
		assert(gmcp_all[1].package == "Core.Ping" and gmcp_all[1].data == nil)
		-- raw must survive a nil data in the middle of the arg list
		assert(gmcp_all[1].raw == "", "raw truncated after nil data")
		assert(gmcp_all[2].data.chan == "gossip")
		assert(gmcp_all[2].raw == '{"chan":"gossip"}')
	`
	if err := engine.DoString("check", check); err != nil {
		t.Fatalf("catch-all hook check failed: %v", err)
	}
}

// TestGMCPMalformedJSONDroppedAndReported verifies a broken server
// message is reported through the error path and never reaches
// handlers - and that the VM keeps working.
func TestGMCPMalformedJSONDroppedAndReported(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		gmcp_count = 0
		rune.gmcp.on("Char.Vitals", function() gmcp_count = gmcp_count + 1 end)
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatal(err)
	}

	engine.OnGMCP("Char.Vitals", `{not json`)

	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if !strings.Contains(printed, "malformed JSON") {
		t.Errorf("expected malformed JSON report, got: %s", printed)
	}
	if err := engine.DoString("check", `assert(gmcp_count == 0, "handler ran on bad JSON")`); err != nil {
		t.Error(err)
	}

	// Valid message afterwards still dispatches
	engine.OnGMCP("Char.Vitals", `{"hp":1}`)
	if err := engine.DoString("check2", `assert(gmcp_count == 1)`); err != nil {
		t.Error(err)
	}
}

// TestGMCPHandshakeAndSubscriptions verifies the gmcp_enabled hook
// sends Core.Hello (with the real version) and Core.Supports.Set, and
// that subscribing while enabled re-sends the full set.
func TestGMCPHandshakeAndSubscriptions(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("subscribe", `rune.gmcp.subscribe("Char")`); err != nil {
		t.Fatal(err)
	}

	engine.CallHook("gmcp_enabled")

	if len(host.GMCPSends) < 2 {
		t.Fatalf("expected Core.Hello + Core.Supports.Set, got %v", host.GMCPSends)
	}
	hello := host.GMCPSends[0]
	if hello.Package != "Core.Hello" ||
		!strings.Contains(hello.Data, `"client":"Rune"`) ||
		!strings.Contains(hello.Data, `"version":"`+version.Number+`"`) {
		t.Errorf("Core.Hello = %+v", hello)
	}
	supports := host.GMCPSends[1]
	if supports.Package != "Core.Supports.Set" || supports.Data != `["Char 1"]` {
		t.Errorf("Core.Supports.Set = %+v", supports)
	}

	// Subscribing while enabled re-sends the whole set
	host.GMCPSends = nil
	if err := engine.DoString("subscribe2", `rune.gmcp.subscribe("Room", 2)`); err != nil {
		t.Fatal(err)
	}
	if len(host.GMCPSends) != 1 || host.GMCPSends[0].Data != `["Char 1","Room 2"]` {
		t.Errorf("re-sent supports = %v", host.GMCPSends)
	}

	// is_enabled resets on disconnect
	engine.CallHook("disconnected")
	if err := engine.DoString("check", `assert(rune.gmcp.is_enabled() == false)`); err != nil {
		t.Error(err)
	}
}

// TestGMCPFailingHandlerQuarantined verifies a throwing GMCP handler
// is disabled after repeated failures instead of erroring forever.
func TestGMCPFailingHandlerQuarantined(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		ok_count = 0
		rune.gmcp.on("Char.Vitals", function() error("boom") end, { name = "bad" })
		rune.gmcp.on("Char.Vitals", function() ok_count = ok_count + 1 end, { name = "good" })
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 4; i++ {
		engine.OnGMCP("Char.Vitals", `{}`)
	}

	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if !strings.Contains(printed, "disabled after 3 consecutive errors") {
		t.Errorf("expected quarantine notice, got:\n%s", printed)
	}
	// The healthy handler ran every time despite its broken sibling
	if err := engine.DoString("check", `assert(ok_count == 4, "good handler starved: " .. ok_count)`); err != nil {
		t.Error(err)
	}
}

// TestGMCPSendEncodesLuaValues verifies rune.gmcp.send round-trips a
// Lua table through the JSON bridge, and failures echo instead of raise.
func TestGMCPSendEncodesLuaValues(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("send", `rune.gmcp.send("Char.Login", { name = "drake" })`); err != nil {
		t.Fatal(err)
	}
	if len(host.GMCPSends) != 1 || host.GMCPSends[0].Package != "Char.Login" ||
		host.GMCPSends[0].Data != `{"name":"drake"}` {
		t.Errorf("sends = %v", host.GMCPSends)
	}

	host.GMCPErr = errNotConnected
	if err := engine.DoString("sendfail", `
		local ok, err = rune.gmcp.send("Core.Ping")
		assert(ok == nil and err ~= nil)
	`); err != nil {
		t.Fatalf("send failure raised instead of returning: %v", err)
	}
	if printed := strings.Join(host.DrainPrintCalls(), "\n"); !strings.Contains(printed, "not connected") {
		t.Errorf("expected send failure echoed, got: %s", printed)
	}
}
