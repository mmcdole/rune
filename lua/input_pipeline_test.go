package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
)

func TestMalformedInputHookResultRejectsLine(t *testing.T) {
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

// dispatchTestInputLine exercises the Engine's one-line hook/dispatch boundary.
// Submission ordering, budgets, history, and cancellation are tested in Session.
func dispatchTestInputLine(engine *Engine, text string, mode input.SubmissionMode) bool {
	if strings.ContainsAny(text, "\r\n") {
		panic("Lua test helper requires one physical line")
	}
	line, proceed, err := engine.ApplyInputHooks(text, mode)
	if err != nil {
		engine.reportError("input", err)
		return false
	}
	if !proceed {
		return false
	}
	if err := engine.DispatchInputLine(line, mode); err != nil {
		engine.reportError("input", err)
	}
	return true
}

func dispatchTestCommand(engine *Engine, text string) bool {
	return dispatchTestInputLine(engine, text, input.ModeCommand)
}

func TestInputLineRejectsInvalidRewrites(t *testing.T) {
	for _, tc := range []struct{ name, setup string }{
		{"terminal control", `rune.hooks.on("input", function() return string.char(27) end)`},
		{"multiline", `rune.hooks.on("input", function() return "north\nsouth" end)`},
		{"oversized", `rune.hooks.on("input", function() return string.rep("x", 256 * 1024 + 1) end)`},
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
