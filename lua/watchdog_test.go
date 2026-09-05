package lua

import (
	"strings"
	"testing"
	"time"
)

// runawayLoop returns an infinite loop the active backend's watchdog
// can actually interrupt. Lunar polls its installed context at bounded
// safe points, including loop back edges, so a bare loop works. LuaJIT compiles a bare loop into
// a trace that never polls debug hooks — that escape is a documented
// backend caveat (docs/luajit.md) — but any loop touching a host
// function stays interpreter-bound (traces abort on C calls), which is
// the realistic runaway class in a scripted client, and is what the
// LuaJIT watchdog is tested against.
func runawayLoop(engine *Engine) string {
	if engine.EngineBackend() == "luajit" {
		return `while true do rune._strip_ansi("x") end`
	}
	return "while true do end"
}

// TestWatchdogInterruptsRunawayScript verifies that a script stuck in an
// infinite loop is interrupted after CallTimeout instead of hanging the
// calling goroutine forever.
func TestWatchdogInterruptsRunawayScript(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	engine.CallTimeout = 100 * time.Millisecond

	start := time.Now()
	err := engine.DoString("runaway", runawayLoop(engine))
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

	if err := engine.DoString("runaway", runawayLoop(engine)); err == nil {
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
	err := engine.DoString("runaway", "rune.input.open_editor(''); "+runawayLoop(engine))
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("watchdog not re-armed after pause: %v", err)
	}
}

// TestWatchdogRunawayHookDoesNotHang verifies the watchdog also covers
// the pre-commit input-hook path, not just direct script execution.
func TestWatchdogRunawayHookDoesNotHang(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.CallTimeout = 100 * time.Millisecond

	setup := "rune.hooks.on('input', function() " + runawayLoop(engine) + " end, {priority = 1})"
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	done := make(chan bool, 1)
	go func() {
		done <- dispatchTestCommand(engine, "north")
	}()

	select {
	case proceed := <-done:
		if proceed {
			t.Fatal("runaway input hook failed open")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("input hook dispatch hung despite watchdog")
	}
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Fatalf("runaway input hook dispatched authored input: %q", sent)
	}
	reported := false
	for _, line := range host.DrainPrintCalls() {
		reported = reported || strings.Contains(line, "interrupted")
	}
	if !reported {
		t.Fatal("runaway input hook cancellation was not reported")
	}
}
