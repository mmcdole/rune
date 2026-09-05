package lua

import (
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
)

func TestMissingInputDispatcherUsesGoFallback(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("remove dispatcher", `rune.input._dispatch = nil`); err != nil {
		t.Fatal(err)
	}

	dispatchTestCommand(engine, "north")
	dispatchTestSubmission(engine, input.Verbatim("one\r\ntwo"))
	dispatchTestCommand(engine, "/quit")
	dispatchTestCommand(engine, "/reload")

	if got, want := host.DrainNetworkCalls(), []string{"north", "one", "two"}; !slices.Equal(got, want) {
		t.Fatalf("fallback sends = %q, want %q", got, want)
	}
	if !host.QuitCalled || host.ReloadCalls != 1 {
		t.Fatalf("fallback escape hatches: quit=%v reload=%d", host.QuitCalled, host.ReloadCalls)
	}
}

func TestFailingInputDispatcherIsNotRetried(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("broken dispatcher", `
		function rune.input._dispatch(text)
			rune.send_raw(text .. ":once")
			error("dispatch failed after send")
		end
	`); err != nil {
		t.Fatal(err)
	}

	dispatchTestCommand(engine, "north")
	if got, want := host.DrainNetworkCalls(), []string{"north:once"}; !slices.Equal(got, want) {
		t.Fatalf("dispatcher sends = %q, want no fallback duplicate %q", got, want)
	}
}

func TestMalformedInputHookResultCancelsSubmission(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("malformed hooks", `
		function rune.hooks.call()
			return true
		end
	`); err != nil {
		t.Fatal(err)
	}

	if dispatchTestCommand(engine, "north") {
		t.Fatal("malformed input-hook result was accepted")
	}
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Fatalf("malformed input-hook result sent %q", sent)
	}
	warned := false
	for _, line := range host.DrainPrintCalls() {
		warned = warned || strings.Contains(line, "core Lua pipeline is incomplete")
	}
	if !warned {
		t.Fatal("malformed input-hook result produced no visible warning")
	}
}

// dispatchTestSubmission drives the two phases that Session separates with
// echo and history commits. Tests focused on Lua routing do not need to model
// those Session-owned commits.
func dispatchTestSubmission(engine *Engine, submission input.Submission) bool {
	effective, proceed := engine.ApplyInputHooks(submission)
	if proceed {
		engine.DispatchSubmission(effective)
	}
	return proceed
}

func dispatchTestCommand(engine *Engine, text string) bool {
	return dispatchTestSubmission(engine, input.Command(text))
}
