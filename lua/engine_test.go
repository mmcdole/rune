package lua

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/drake/rune/text"
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

// TestFailingBarIsRemoved verifies that a bar renderer failing
// repeatedly is removed instead of erroring 4x/second forever.
func TestFailingBarIsRemoved(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	setup := `rune.ui.bar("hp", function() error("boom") end)`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		engine.RenderBar("hp", 80)
	}
	if engine.HasBar("hp") {
		t.Error("expected failing bar to be removed after 3 errors")
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
