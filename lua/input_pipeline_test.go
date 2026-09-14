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
	dispatchTestLine(engine, input.Line{Text: "/quit", Mode: input.ModeVerbatim})
	dispatchTestCommand(engine, "/quit")
	dispatchTestCommand(engine, "/reload")

	if got, want := host.DrainNetworkCalls(), []string{"north", "/quit"}; !slices.Equal(got, want) {
		t.Fatalf("fallback sends = %q, want %q", got, want)
	}
	if !host.QuitCalled || host.ReloadCalls != 1 {
		t.Fatalf("fallback escape hatches: quit=%v reload=%d", host.QuitCalled, host.ReloadCalls)
	}
}

func TestMalformedInputHookResultCancelsLine(t *testing.T) {
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

func TestInputLineRejectsInvalidRewrites(t *testing.T) {
	for _, tc := range []struct{ name, setup string }{
		{"terminal control", `rune.hooks.on("input", function() return string.char(27) end)`},
		{"multiline", `rune.hooks.on("input", function() return "north\nsouth" end)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString(tc.name, tc.setup); err != nil {
				t.Fatal(err)
			}
			if dispatchTestCommand(engine, "north") {
				t.Fatal("invalid rewrite accepted")
			}
			if got := host.DrainNetworkCalls(); len(got) != 0 {
				t.Fatalf("sent %q", got)
			}
		})
	}
}
