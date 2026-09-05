package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// TestBrokenHooksDegradesGracefully verifies that destroying rune.hooks does
// not crash the client: output passes through raw while the independent input
// dispatcher continues routing commands.
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
	dispatchTestCommand(engine, "north")
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "north" {
		t.Errorf("expected raw send of input, got %v", sent)
	}

	// Escape hatches still work
	dispatchTestCommand(engine, "/quit")
	if !host.QuitCalled {
		t.Error("expected /quit to reach host in degraded mode")
	}
	dispatchTestCommand(engine, "/reload")
	if host.ReloadCalls != 1 {
		t.Errorf("expected /reload to reach host in degraded mode, got %d calls", host.ReloadCalls)
	}

	// The warning is printed exactly once
	warnings := 0
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "core Lua pipeline is incomplete") {
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
// must not disable the shared input pipeline (including /reload).
func TestFailingCommandIsQuarantinedIndividually(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `rune.command.add("badcmd", function() error("boom") end, "broken")`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		dispatchTestCommand(engine, "/badcmd")
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
	dispatchTestCommand(engine, "north")
	sent := host.DrainNetworkCalls()
	if len(sent) != 1 || sent[0] != "north" {
		t.Fatalf("input pipeline dead after command quarantine: sent %v", sent)
	}

	dispatchTestCommand(engine, "/badcmd")
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
