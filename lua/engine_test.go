package lua

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/version"
)

var errNotConnected = errors.New("not connected")

// TestWatchdogInterruptsRunawayScript verifies that a script stuck in an
// infinite loop is interrupted after CallTimeout instead of hanging the
// calling goroutine forever.
func TestWatchdogInterruptsRunawayScript(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	engine.CallTimeout = 100 * time.Millisecond

	start := time.Now()
	err := engine.DoString("runaway", "while true do end")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected runaway script to be interrupted, got nil error")
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("expected watchdog error, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("interruption took %v, expected roughly CallTimeout", elapsed)
	}
}

// TestWatchdogStateUsableAfterInterrupt verifies the VM survives an
// interrupted script and continues to execute normally.
func TestWatchdogStateUsableAfterInterrupt(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.CallTimeout = 100 * time.Millisecond

	if err := engine.DoString("runaway", "while true do end"); err == nil {
		t.Fatal("expected runaway script to be interrupted")
	}

	if err := engine.DoString("after", `rune.send_raw("still alive")`); err != nil {
		t.Fatalf("VM unusable after interrupt: %v", err)
	}
	sent := host.DrainNetworkCalls()
	if len(sent) != 1 || sent[0] != "still alive" {
		t.Errorf("expected send after interrupt, got %v", sent)
	}
}

// TestBrokenHooksDegradesGracefully verifies that destroying rune.hooks
// from a user script does not crash the client: output passes through
// raw, input goes to the server, and the escape hatches keep working.
func TestBrokenHooksDegradesGracefully(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("sabotage", "rune.hooks = nil"); err != nil {
		t.Fatalf("sabotage failed: %v", err)
	}

	// Output passes through unmodified
	line := text.NewLine("You are standing in a field.")
	got, show := engine.OnOutput(line)
	if !show || got != line.Raw {
		t.Errorf("expected raw pass-through, got %q show=%v", got, show)
	}

	// Input goes straight to the server
	engine.OnInput("north")
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "north" {
		t.Errorf("expected raw send of input, got %v", sent)
	}

	// Escape hatches still work
	engine.OnInput("/quit")
	if !host.QuitCalled {
		t.Error("expected /quit to reach host in degraded mode")
	}
	engine.OnInput("/reload")
	if host.ReloadCalls != 1 {
		t.Errorf("expected /reload to reach host in degraded mode, got %d calls", host.ReloadCalls)
	}

	// The warning is printed exactly once
	warnings := 0
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "rune.hooks.call is unavailable") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("expected exactly one degraded-mode warning, got %d", warnings)
	}
}

// TestBrokenHandlerIsIsolated verifies that a throwing output handler
// is reported and skipped, while later handlers (including core trigger
// processing) still run.
func TestBrokenHandlerIsIsolated(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		rune.hooks.on("output", function() error("boom") end, {priority = 10, name = "bad"})
		rune.trigger.contains("field", "look")
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	got, show := engine.OnOutput(text.NewLine("You are standing in a field."))
	if !show || got != "You are standing in a field." {
		t.Errorf("expected line to pass through, got %q show=%v", got, show)
	}

	// Trigger registered after the broken handler still fired
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "look" {
		t.Errorf("expected trigger to fire despite broken handler, got %v", sent)
	}

	// The handler's error was reported with its name
	reported := false
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, `"bad"`) && strings.Contains(p, "boom") {
			reported = true
		}
	}
	if !reported {
		t.Error("expected broken handler error to be echoed")
	}
}

// TestFailingHookIsQuarantined verifies that a handler failing on every
// line is disabled after the failure limit, stopping the error spam.
func TestFailingHookIsQuarantined(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `rune.hooks.on("output", function() error("boom") end, {name = "bad"})`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		engine.OnOutput(text.NewLine("a line"))
	}

	disabled := false
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "disabled after 3 consecutive errors") {
			disabled = true
		}
	}
	if !disabled {
		t.Fatal("expected quarantine notice after 3 failures")
	}

	// A quarantined handler no longer runs or reports
	engine.OnOutput(text.NewLine("another line"))
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "boom") {
			t.Errorf("quarantined handler still reporting: %q", p)
		}
	}
}

// TestFailingCommandIsQuarantinedIndividually verifies that a slash
// command throwing repeatedly is disabled by itself - its failures
// must not accrue against the core input hook, which would take the
// whole input pipeline (including /reload) down with it.
func TestFailingCommandIsQuarantinedIndividually(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `rune.command.add("badcmd", function() error("boom") end, "broken")`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		engine.OnInput("/badcmd")
	}

	disabled := false
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "disabled after 3 consecutive errors") {
			disabled = true
		}
	}
	if !disabled {
		t.Fatal("expected quarantine notice after 3 command failures")
	}

	// The input pipeline must still be alive: plain input sends, and
	// other slash commands still dispatch.
	engine.OnInput("north")
	sent := host.DrainNetworkCalls()
	if len(sent) != 1 || sent[0] != "north" {
		t.Fatalf("input pipeline dead after command quarantine: sent %v", sent)
	}

	engine.OnInput("/badcmd")
	sawDisabled := false
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "is disabled") {
			sawDisabled = true
		}
		if strings.Contains(p, "boom") {
			t.Errorf("quarantined command still running: %q", p)
		}
	}
	if !sawDisabled {
		t.Error("expected disabled notice when invoking a quarantined command")
	}
}

// TestEchoHookStylesAndGags verifies the "echo" event: the core
// handler styles the echo, and a user handler returning false hides
// it entirely.
func TestEchoHookStylesAndGags(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	styled, show := engine.OnEcho("north")
	if !show {
		t.Fatal("default echo unexpectedly hidden")
	}
	if !strings.Contains(styled, "> north") {
		t.Errorf("expected core styling with '> ' prefix, got %q", styled)
	}

	gag := `rune.hooks.on("echo", function(text)
		if text == "secret" then return false end
	end, {priority = 1})`
	if err := engine.DoString("gag", gag); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if _, show := engine.OnEcho("secret"); show {
		t.Error("echo hook returning false should hide the echo")
	}
	if _, show := engine.OnEcho("hello"); !show {
		t.Error("non-matching input should still echo")
	}
}

// TestConnectCommandForms verifies the /connect argument shapes:
// host+port, host+port+tls scheme, and the single host:port form.
func TestConnectCommandForms(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	cases := []struct {
		input string
		want  string // expected address, "" = usage error instead
	}{
		{"/connect mud.example.com 4000", "mud.example.com:4000"},
		{"/connect mud.example.com 4000 tls", "tls://mud.example.com:4000"},
		{"/connect mud.example.com 4000 tls+insecure", "tls+insecure://mud.example.com:4000"},
		{"/connect mud.example.com:4000", "mud.example.com:4000"},
		{"/connect tls://mud.example.com:4000", "tls://mud.example.com:4000"},
		{"/connect mud.example.com 4000 bogus", ""},
		{"/connect mud.example.com", ""},
	}
	for _, c := range cases {
		host.ConnectCalls = nil
		engine.OnInput(c.input)
		if c.want == "" {
			if len(host.ConnectCalls) != 0 {
				t.Errorf("%q: expected usage error, connected to %v", c.input, host.ConnectCalls)
			}
			continue
		}
		if len(host.ConnectCalls) != 1 || host.ConnectCalls[0] != c.want {
			t.Errorf("%q: connect calls = %v, want [%s]", c.input, host.ConnectCalls, c.want)
		}
	}
}

// TestFailingBarIsQuarantined verifies that a bar renderer failing
// repeatedly is disabled instead of erroring 4x/second forever, and
// that re-registering it gives a fresh start.
func TestFailingBarIsQuarantined(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `rune.ui.bar("hp", function() error("boom") end)`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		engine.RenderBars(80)
	}

	disabled := false
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "disabled after 3 consecutive errors") {
			disabled = true
		}
	}
	if !disabled {
		t.Fatal("expected quarantine notice after 3 failures")
	}

	// The quarantined bar no longer renders or reports
	if content := engine.RenderBars(80); content != nil {
		if _, ok := content["hp"]; ok {
			t.Error("quarantined bar still rendering")
		}
	}
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "boom") {
			t.Errorf("quarantined bar still reporting: %q", p)
		}
	}

	// Re-registering the bar resets the quarantine
	if err := engine.DoString("rereg", `rune.ui.bar("hp", function() return "HP 100" end)`); err != nil {
		t.Fatalf("re-register failed: %v", err)
	}
	content := engine.RenderBars(80)
	if content == nil || content["hp"].Left != "HP 100" {
		t.Errorf("re-registered bar did not render, got %v", content)
	}
}

// TestInvalidRegexFailsAtRegistration verifies that a bad pattern is a
// loud error at trigger/alias creation, not a trigger that never fires.
func TestInvalidRegexFailsAtRegistration(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("setup", `rune.trigger.regex("(unclosed", "look")`)
	if err == nil || !strings.Contains(err.Error(), "invalid trigger pattern") {
		t.Errorf("expected registration error for bad trigger pattern, got: %v", err)
	}

	err = engine.DoString("setup2", `rune.alias.regex("(unclosed", "look")`)
	if err == nil || !strings.Contains(err.Error(), "invalid alias pattern") {
		t.Errorf("expected registration error for bad alias pattern, got: %v", err)
	}
}

// TestCaptureWithPercentIsLiteral verifies that captured text containing
// "%" is substituted literally instead of corrupting the gsub template.
func TestCaptureWithPercentIsLiteral(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `rune.trigger.regex("^You gain (\\S+) exp", "say %1")`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	engine.OnOutput(text.NewLine("You gain 50% exp"))
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "say 50%" {
		t.Errorf("expected literal %%-capture substitution, got %v", sent)
	}
}

// TestSendRawFailureIsReportedNotRaised verifies the nil+err convention:
// a failed send is echoed and returned as a value, and does not raise a
// Lua error that would abort the calling script.
func TestSendRawFailureIsReportedNotRaised(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.SendErr = errNotConnected

	script := `
		local ok, err = rune.send_raw("north")
		assert(ok == nil, "expected nil ok")
		assert(err == "not connected", "expected error message, got " .. tostring(err))
	`
	if err := engine.DoString("test", script); err != nil {
		t.Fatalf("send_raw should not raise: %v", err)
	}

	echoed := false
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "not connected") {
			echoed = true
		}
	}
	if !echoed {
		t.Error("expected send failure to be echoed")
	}
}

// TestRegistrationsRecordSource verifies that hooks, triggers, aliases,
// and timers record the registering script's file:line for listings and
// error attribution.
func TestRegistrationsRecordSource(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	script := `
		rune.hooks.on("output", function() end, {name = "h"})
		rune.trigger.contains("x", "look", {name = "t"})
		rune.alias.exact("n", "north", {name = "a"})
		rune.timer.after(60, function() end, {name = "tm"})

		local function source_of(list, name)
			for _, item in ipairs(list) do
				if item.name == name then
					return item.source
				end
			end
		end

		for _, entry in ipairs({
			{"hook", rune.hooks.list(), "h"},
			{"trigger", rune.trigger.list(), "t"},
			{"alias", rune.alias.list(), "a"},
			{"timer", rune.timer.list(), "tm"},
		}) do
			local src = source_of(entry[2], entry[3])
			assert(src and src:find("attr_test"),
				entry[1] .. " source not recorded, got " .. tostring(src))
		end
	`
	if err := engine.DoString("attr_test.lua", script); err != nil {
		t.Fatalf("source attribution failed: %v", err)
	}
}

// TestSessionStoreSurvivesReload verifies rune.session round-trips
// through the host and survives a VM teardown (the whole point).
func TestSessionStoreSurvivesReload(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	script := `
		rune.session.set("last_address", "mud.example.com:4000")
		assert(rune.session.get("last_address") == "mud.example.com:4000")
		assert(rune.session.get("missing") == nil)
		rune.session.set("gone", "x")
		rune.session.delete("gone")
		assert(rune.session.get("gone") == nil)
	`
	if err := engine.DoString("session_test", script); err != nil {
		t.Fatalf("session store round-trip failed: %v", err)
	}

	// Tear down and rebuild the VM, as /reload does
	if err := engine.Init(); err != nil {
		t.Fatalf("re-init failed: %v", err)
	}
	if host.SessionStore["last_address"] != "mud.example.com:4000" {
		t.Error("session store value lost across VM teardown")
	}
}

// TestStoreRoundTripsStructuredValues verifies rune.store converts
// Lua values to JSON and back: nested tables, arrays, scalars; and
// that unstorable values are rejected with nil, err instead of raised.
func TestStoreRoundTripsStructuredValues(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	script := `
		assert(rune.store.set("cfg", {
			name = "arctic",
			hp_warn = 0.25,
			auto = true,
			route = {"n", "e", "e"},
			nested = { deep = { value = 42 } },
		}))

		local cfg = rune.store.get("cfg")
		assert(cfg.name == "arctic")
		assert(cfg.hp_warn == 0.25)
		assert(cfg.auto == true)
		assert(#cfg.route == 3 and cfg.route[2] == "e")
		assert(cfg.nested.deep.value == 42)

		assert(rune.store.get("missing") == nil)

		-- Unstorable: functions
		local ok, err = rune.store.set("bad", { fn = function() end })
		assert(ok == nil and err ~= nil, "function value should be rejected")

		-- Unstorable: cycles
		local cyc = {}
		cyc.self = cyc
		local ok2, err2 = rune.store.set("bad", cyc)
		assert(ok2 == nil and err2 ~= nil, "cycle should be rejected")

		-- set(key, nil) deletes
		assert(rune.store.set("cfg", nil))
		assert(rune.store.get("cfg") == nil)
	`
	if err := engine.DoString("store_test", script); err != nil {
		t.Fatalf("store round-trip failed: %v", err)
	}
}

// TestWorldResolution verifies /world add saves a bookmark and
// /connect resolves the name before falling back to address parsing.
func TestWorldResolution(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnInput("/world add arctic mud.arcticmud.org 2700")
	engine.OnInput("/world add secure example.com 4000 tls")
	engine.OnInput("/connect arctic")
	engine.OnInput("/connect secure")

	want := []string{"mud.arcticmud.org:2700", "tls://example.com:4000"}
	if len(host.ConnectCalls) != 2 || host.ConnectCalls[0] != want[0] || host.ConnectCalls[1] != want[1] {
		t.Fatalf("connect calls = %v, want %v", host.ConnectCalls, want)
	}

	// Removed worlds no longer resolve (and a bare name is not an address)
	engine.OnInput("/world remove arctic")
	host.ConnectCalls = nil
	engine.OnInput("/connect arctic")
	if len(host.ConnectCalls) != 0 {
		t.Errorf("removed world still resolved: %v", host.ConnectCalls)
	}
}

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

// TestVersionSingleSourced verifies rune.version comes from the Go
// version package (the same value TTYPE/MNES report).
func TestVersionSingleSourced(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("check",
		`assert(rune.version == "`+version.Number+`", "rune.version = " .. tostring(rune.version))`); err != nil {
		t.Error(err)
	}
}

// TestReconnectUsesStoredAddress verifies the "connected" handler
// stores the address durably and /reconnect dials it again.
func TestReconnectUsesStoredAddress(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.CallHook("connected", "tls://mud.example.com:4000")
	engine.OnInput("/reconnect")

	if len(host.ConnectCalls) != 1 || host.ConnectCalls[0] != "tls://mud.example.com:4000" {
		t.Errorf("connect calls = %v, want the stored address (scheme intact)", host.ConnectCalls)
	}
}

// TestKeyBindRoundTrip verifies the bind path: Lua registers through
// rune.bind, Go sees the key via GetBoundKeys, and HandleKeyBind
// dispatches back into the Lua callback. Unbinding removes the key.
func TestKeyBindRoundTrip(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `rune.bind("ctrl+g", function() rune.send_raw("bound") end)`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	found := false
	for _, k := range engine.GetBoundKeys() {
		if k == "ctrl+g" {
			found = true
		}
	}
	if !found {
		t.Fatal("ctrl+g missing from GetBoundKeys")
	}

	engine.HandleKeyBind("ctrl+g")
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "bound" {
		t.Errorf("expected bind callback to fire, got %v", sent)
	}

	// An unbound key is a no-op, not an error
	engine.HandleKeyBind("ctrl+q")
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("unbound key fired something: %v", sent)
	}

	if err := engine.DoString("unbind", `assert(rune.unbind("ctrl+g"))`); err != nil {
		t.Fatalf("unbind failed: %v", err)
	}
	for _, k := range engine.GetBoundKeys() {
		if k == "ctrl+g" {
			t.Error("ctrl+g still bound after unbind")
		}
	}
}

// TestTimerDispatchRoundTrip verifies the full timer path: Lua
// schedules through the Go primitive, Go wakes the engine with the
// id, and the Lua module dispatches to the right callback. Stale ids
// (from cancelled timers or a previous VM) must be ignored.
func TestTimerDispatchRoundTrip(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		rune.timer.after(60, function() rune.send_raw("fired") end, {name = "tm"})
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	scheduled := host.DrainScheduledTimers()
	if len(scheduled) != 1 {
		t.Fatalf("expected 1 scheduled timer, got %d", len(scheduled))
	}

	// A stale id does nothing
	engine.OnTimer(scheduled[0].ID + 999)
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("stale timer id fired a callback: %v", sent)
	}

	engine.OnTimer(scheduled[0].ID)
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "fired" {
		t.Errorf("expected timer callback to fire, got %v", sent)
	}

	// One-shot: firing removed it from the registry and the id map
	engine.OnTimer(scheduled[0].ID)
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("one-shot fired twice: %v", sent)
	}
	if err := engine.DoString("check", `assert(rune.timer.count() == 0, "timer not removed")`); err != nil {
		t.Errorf("one-shot not removed after firing: %v", err)
	}
}

// TestWatchdogPausedDuringBlockingHostCall verifies that time spent in
// a blocking host call (the user sitting in $EDITOR) does not count
// against the watchdog deadline: the handler must survive an editor
// session longer than CallTimeout and keep its result.
func TestWatchdogPausedDuringBlockingHostCall(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.CallTimeout = 100 * time.Millisecond
	host.OpenEditorFn = func(initial string) (string, bool) {
		time.Sleep(300 * time.Millisecond) // longer than CallTimeout
		return "edited text", true
	}

	script := `
		local result, ok = rune.input.open_editor("draft")
		assert(ok, "editor result lost")
		rune.send_raw(result)
	`
	if err := engine.DoString("editor_test", script); err != nil {
		t.Fatalf("handler killed after blocking host call: %v", err)
	}
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "edited text" {
		t.Errorf("expected edited text to survive, got %v", sent)
	}

	// The re-armed deadline must still catch a runaway loop afterwards.
	err := engine.DoString("runaway", "rune.input.open_editor(''); while true do end")
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("watchdog not re-armed after pause: %v", err)
	}
}

// TestWatchdogRunawayHookDoesNotHang verifies the watchdog also covers
// the hook dispatch path (OnInput), not just direct script execution.
func TestWatchdogRunawayHookDoesNotHang(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	engine.CallTimeout = 100 * time.Millisecond

	setup := `rune.hooks.on("input", function() while true do end end, {priority = 1})`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		engine.OnInput("north")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnInput hung despite watchdog")
	}
}
