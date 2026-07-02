package lua

import (
	"strings"
	"testing"
	"time"

	"github.com/drake/rune/text"
)

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
		if strings.Contains(p, "[Hook Error]") && strings.Contains(p, `"bad"`) {
			reported = true
		}
	}
	if !reported {
		t.Error("expected broken handler error to be echoed")
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
