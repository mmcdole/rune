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

	dispatchTestCommand(engine, "north\neast\n\t\nwest")
	dispatchTestSubmission(engine, input.Verbatim("one\r\ntwo"))
	dispatchTestCommand(engine, "/quit")
	dispatchTestCommand(engine, "/reload")

	if got, want := host.DrainNetworkCalls(), []string{"north", "east", "west", "one", "two"}; !slices.Equal(got, want) {
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

func TestCommandBatchStopsOnFailure(t *testing.T) {
	for _, tc := range []struct{ name, setup, draft string }{
		{"unknown command", "", "north\n/missing\nsouth"},
		{"throwing command", `rune.command.add("broken", function() error("broken") end)`, "north\n/broken\nsouth"},
		{"throwing alias", `rune.alias.exact("broken", function() error("broken") end)`, "north\nbroken\nsouth"},
		{"invalid Lua", "", "north\n/lua invalid lua syntax\nsouth"},
		{"Lua runtime error", "", "north\n/lua error('broken')\nsouth"},
		{"alias recursion", `rune.alias.exact("loop", "loop")`, "north\nloop\nsouth"},
		{"dispatcher throws after send", `function rune.input._dispatch(line) rune.send_raw(line); error("broken") end`, "north\nsouth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString(tc.name, tc.setup); err != nil {
				t.Fatal(err)
			}
			dispatchTestCommand(engine, tc.draft)
			if got := host.DrainNetworkCalls(); !slices.Equal(got, []string{"north"}) {
				t.Fatalf("sent %q", got)
			}
			if got := strings.Join(host.DrainPrintCalls(), "\n"); !strings.Contains(got, "line ") {
				t.Fatalf("missing line number: %s", got)
			}
		})
	}
}

func TestCommandBatchValidatesBeforeDispatch(t *testing.T) {
	for _, tc := range []struct{ name, setup, draft string }{
		{"terminal control", "", "north\nsouth\x1b"},
		{"missing history", "", "north\n!missing"},
		{"invalid rewrite", `rune.hooks.on("input", function(line) if line == "south" then return string.char(27) end end)`, "north\nsouth"},
		{"oversized rewrite", `rune.hooks.on("input", function() return string.rep("x", 256 * 1024 + 1) end)`, "north"},
		{"combined rewrite limit", `rune.hooks.on("input", function() return string.rep("x", 140 * 1024) end)`, "north\nsouth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString(tc.name, tc.setup); err != nil {
				t.Fatal(err)
			}
			if dispatchTestCommand(engine, tc.draft) {
				t.Fatal("invalid batch accepted")
			}
			if got := host.DrainNetworkCalls(); len(got) != 0 {
				t.Fatalf("sent %q", got)
			}
		})
	}
}

func TestCommandBatchHooksAndHistoryExpansion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.HistoryEntries = []input.Submission{input.Command("look\nscore\n/echo ignored")}
	if err := engine.DoString("batch hooks", `
		seen = {}
		rune.hooks.on("input", function(line, context)
			assert(context.mode == "command")
			assert(#rune.history.get() == 1)
			seen[#seen + 1] = line
			if line == "rewrite" then return "say one;;two\n!" end
		end, {priority = 50})
	`); err != nil {
		t.Fatal(err)
	}
	dispatchTestCommand(engine, "!\n/echo local\nrewrite\n\t\n!look")
	if got := host.DrainNetworkCalls(); !slices.Equal(got, []string{"score", "say one;two", "score", "look"}) {
		t.Fatalf("sent %q", got)
	}
	assertLua(t, engine, `assert(table.concat(seen, "|") == "!|/echo local|rewrite|!look")`)
}
