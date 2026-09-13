package lua

import (
	"errors"
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

	dispatchTestCommand(engine, "north\nsouth")
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

// Lua-focused tests drive lines without Session's echo, history, or budget policy.
// Production batch semantics are covered by session/submission_test.go.
func dispatchTestSubmission(engine *Engine, submission input.Submission) bool {
	end := engine.BeginBatch()
	defer end()
	accepted := false
	for _, authored := range submission.Lines() {
		line, proceed, err := engine.ApplyInputHooks(authored, submission.Mode)
		if err != nil {
			engine.reportError("input", err)
			if errors.Is(err, ErrInterrupted) {
				break
			}
			continue
		}
		if !proceed {
			continue
		}
		accepted = true
		if err := engine.DispatchInputLine(line, submission.Mode); err != nil {
			engine.reportError("input", err)
			break
		}
	}
	return accepted
}

func dispatchTestCommand(engine *Engine, text string) bool {
	return dispatchTestSubmission(engine, input.Command(text))
}

func TestCommandBatchContinuesAfterCommandErrors(t *testing.T) {
	for _, tc := range []struct{ name, setup, draft string }{
		{"unknown command", "", "north\n/missing\nsouth"},
		{"throwing command", `rune.command.add("broken", function() error("broken") end)`, "north\n/broken\nsouth"},
		{"throwing alias", `rune.alias.exact("broken", function() error("broken") end)`, "north\nbroken\nsouth"},
		{"invalid Lua", "", "north\n/lua invalid lua syntax\nsouth"},
		{"Lua runtime error", "", "north\n/lua error('broken')\nsouth"},
		{"alias recursion", `rune.alias.exact("loop", "loop")`, "north\nloop\nsouth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString(tc.name, tc.setup); err != nil {
				t.Fatal(err)
			}
			dispatchTestCommand(engine, tc.draft)
			if got := host.DrainNetworkCalls(); !slices.Equal(got, []string{"north", "south"}) {
				t.Fatalf("sent %q", got)
			}
			if got := strings.Join(host.DrainPrintCalls(), "\n"); got == "" {
				t.Fatalf("missing error: %s", got)
			}
		})
	}
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
			if line == "rewrite" then return "say one;;two;!" end
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
