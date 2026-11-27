package lua

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/rune/input"
)

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

func TestSendExpansion(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:  "repeated command beginning with braces",
			input: "#2 {north};look",
			want:  []string{"{north}", "{north}", "look"},
		},
		{
			name:  "single command",
			input: "north",
			want:  []string{"north"},
		},
		{
			name:  "multiple commands",
			input: "say hello;east;look",
			want:  []string{"say hello", "east", "look"},
		},
		{
			name:  "extra whitespace",
			input: "  say hello ;  east; look  ",
			want:  []string{"say hello", "east", "look"},
		},
		{
			name:  "empty commands",
			input: ";say hello; ;look;",
			want:  []string{"", "say hello", "", "look", ""},
		},
		{
			name:  "doubled separator is literal",
			input: "say hello;;look;east",
			want:  []string{"say hello;look", "east"},
		},
		{
			name:  "separator pairs are consumed left to right",
			input: "say one;;;say two;;;;three",
			want:  []string{"say one;", "say two;;three"},
		},
		{
			name:  "literal separator at command edges",
			input: ";;look;;",
			want:  []string{";look;"},
		},
		{
			name:  "only a literal separator",
			input: ";;",
			want:  []string{";"},
		},
		{
			name:  "empty input",
			setup: `rune.send("")`,
			want:  []string{""},
		},
		{
			name:  "only whitespace",
			input: "   ",
			want:  []string{""},
		},
		{
			name:  "whitespace between semicolons",
			input: ";   ;   ;",
			want:  []string{"", "", "", ""},
		},
		{
			name:  "repeat at start",
			input: "#3 north",
			want:  []string{"north", "north", "north"},
		},
		{
			name:  "repeat after command separator",
			input: "open gate;#2 south",
			want:  []string{"open gate", "south", "south"},
		},
		{
			name:  "repeat only the next command",
			input: "#2 kill rat;loot",
			want:  []string{"kill rat", "kill rat", "loot"},
		},
		{
			name:  "repeat a literal separator",
			input: "#2 say one;;two",
			want:  []string{"say one;two", "say one;two"},
		},
		{
			name:  "repeat an alias for a command sequence",
			setup: `rune.alias.exact("round", "kill rat;loot")`,
			input: "look;#2 round;west",
			want:  []string{"look", "kill rat", "loot", "kill rat", "loot", "west"},
		},
		{
			name:  "repeat text containing literal braces",
			input: "#2 say {hello}",
			want:  []string{"say {hello}", "say {hello}"},
		},
		{
			name:  "braces need not balance in game text",
			input: "#2 say {hello",
			want:  []string{"say {hello", "say {hello"},
		},
		{
			name:  "zero repeats",
			input: "#0 north;look",
			want:  []string{"look"},
		},
		{
			name:  "repeat shorthand is not a nested language",
			input: "#2 #3 north",
			want:  []string{"#3 north", "#3 north"},
		},
		{
			name:  "escaped separator does not introduce a repeat",
			input: "say hello;;#2 {north;east}",
			want:  []string{"say hello;#2 {north", "east}"},
		},
		{
			name:  "ordinary braces do not quote separators",
			input: "say {one;two}",
			want:  []string{"say {one", "two}"},
		},
		{
			name:  "repeat mid-text passes through",
			input: "say #3 cheers",
			want:  []string{"say #3 cheers"},
		},
		{
			name:  "repeat mid-text with real repeat",
			input: "say meet at #4;#2 west",
			want:  []string{"say meet at #4", "west", "west"},
		},
	})
}

func TestSendEscapesConfiguredSeparator(t *testing.T) {
	for _, separator := range []string{"|", "::", "%", "↻"} {
		t.Run(separator, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			text := "say one" + separator + separator + "two" + separator + "#2 east" + separator + "west"
			if err := engine.DoString("escaped separator", fmt.Sprintf(`
				rune.config.set("command_separator", %q)
				rune.send(%q)
			`, separator, text)); err != nil {
				t.Fatal(err)
			}
			assertCommands(t, host, []string{"say one" + separator + "two", "east", "east", "west"})
		})
	}
}

func TestRepeatedAliasReceivesDecodedArgumentsEachTime(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		local calls = 0
		rune.alias.exact("count", function(args)
			assert(args == "one;two", args)
			calls = calls + 1
			rune.send_raw(calls .. ": " .. args)
		end)
		rune.send("#2 count one;;two")
	`)
	assertCommands(t, host, []string{"1: one;two", "2: one;two"})
}

func TestAliasReceivesLiteralSeparatorForDeferredCommands(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		local deferred
		rune.alias.exact("pendwalk", function(args) deferred = args end)
		rune.send("pendwalk 12345 open desk;;take all desk")
		assert(deferred == "12345 open desk;take all desk", deferred)
		rune.send(deferred:match("^%d+ (.*)$"))
	`)
	assertCommands(t, host, []string{"open desk", "take all desk"})
}

func TestAliasReturnStartsANewCommandParse(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		rune.alias.exact("again", function(args) return args end)
		rune.send("again look;;north")
		rune.send("again say one;;;;two")
		rune.send_raw("say raw;;text")
	`)
	assertCommands(t, host, []string{"look", "north", "say one;two", "say raw;;text"})
}

func TestEscapedSeparatorDoesNotIntroduceRepeatSyntax(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		rune.config.set("command_separator", "#")
		rune.send("##2 north")
	`)
	assertCommands(t, host, []string{"#2 north"})
}

func TestSendExpansionUsesConfiguredCommandSeparator(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("configured command separator repeats", `
		rune.config.set("command_separator", "|")
		rune.send("look|#2 north|#2 east|west|say #3 cheers")
	`); err != nil {
		t.Fatal(err)
	}

	assertCommands(t, host, []string{
		"look", "north", "north", "east", "east", "west", "say #3 cheers",
	})
}

func TestInputWithCommandContextKeepsNormalExpansion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	dispatchTestCommand(engine, "look;#2 north")

	assertCommands(t, host, []string{"look", "north", "north"})
}

func TestInputHooksChainStringsAndReturnOneFinalValue(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		rune.hooks.clear("input")
		assert(rune.hooks.call("input", "unchanged", {mode = "command"}) == "unchanged")

		local seen = {}
		rune.hooks.on("input", function(text)
			seen[#seen + 1] = text
			return text .. "-first"
		end, {name = "rewrite-first", priority = 10})
		rune.hooks.on("input", function(text)
			seen[#seen + 1] = text
			return 42 -- non-string, non-false values pass through
		end, {name = "rewrite-ignore", priority = 20})
		rune.hooks.on("input", function(text)
			seen[#seen + 1] = text
			return text .. "-last"
		end, {name = "rewrite-last", priority = 200})

		local result = rune.hooks.call("input", "raw", {mode = "command"})
		assert(result == "raw-first-last", tostring(result))
		assert(#seen == 3)
		assert(seen[1] == "raw")
		assert(seen[2] == "raw-first")
		assert(seen[3] == "raw-first")
	`)
}

func TestInputHookFalseStopsTransformChain(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		local after = false
		rune.hooks.on("input", function(text)
			return text .. "-changed"
		end, {name = "before-consume", priority = 10})
		rune.hooks.on("input", function(text)
			assert(text == "raw-changed")
			return false
		end, {name = "consume", priority = 20})
		rune.hooks.on("input", function()
			after = true
		end, {name = "after-consume", priority = 30})

		assert(rune.hooks.call("input", "raw", {mode = "command"}) == false)
		assert(after == false)
	`)
}

func TestInputDispatchDoesNotRunInputHooks(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("dispatch", `
		input_hook_calls = 0
		rune.hooks.on("input", function()
			input_hook_calls = input_hook_calls + 1
		end, {name = "dispatch-observer", priority = 1})
		rune.input._execute_input_line("look;#2 north", "command")
		assert(input_hook_calls == 0)
	`); err != nil {
		t.Fatal(err)
	}

	assertCommands(t, host, []string{"look", "north", "north"})
}

func TestInputDispatchVerbatimBypassesCommandSyntax(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("dispatch verbatim", `
		rune.input._execute_input_line("/quit;#2 north", "verbatim")
	`); err != nil {
		t.Fatal(err)
	}

	assertCommands(t, host, []string{"/quit;#2 north"})
	if host.QuitCalled {
		t.Fatal("verbatim dispatcher interpreted /quit")
	}
}

func TestCommandInputHookReceivesContext(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.hooks.on("input", function(text, context)
			rune.send_raw(context.mode .. "|" .. text)
			return false
		end, { priority = 90 })
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	dispatchTestCommand(engine, "look")
	assertCommands(t, host, []string{"command|look"})
}

func TestVerbatimInputHookReceivesContextAndCanConsume(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.hooks.on("input", function(text, context)
			observed_text = text
			observed_mode = context.mode
			return false
		end, { priority = 90 })
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	dispatchTestLine(engine, input.Line{Text: "/quit", Mode: input.ModeVerbatim})

	assertCommands(t, host, nil)
	assertLua(t, engine, `
		assert(observed_mode == "verbatim")
		assert(observed_text == "/quit")
	`)
}

func TestInputHookCannotMutateVerbatimRouting(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.hooks.on("input", function(text, context)
			local writable = pcall(function()
				context.mode = "command"
			end)
			rune.send_raw(writable and "context-writable" or "context-readonly")
			-- rawset can alter this handler's proxy, but every handler receives
			-- a fresh view of the canonical mode.
			rawset(context, "mode", "command")
		end, { priority = 90 })
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	dispatchTestLine(engine, input.Line{Text: "/quit", Mode: input.ModeVerbatim})

	assertCommands(t, host, []string{"context-readonly", "/quit"})
	if host.QuitCalled {
		t.Fatal("mutating one hook context changed canonical verbatim routing")
	}
}

func TestInputRewritePreservesVerbatimMode(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("rewrite verbatim", `
		rune.hooks.on("input", function()
			return "first;second"
		end, { priority = 90 })
	`); err != nil {
		t.Fatal(err)
	}

	effective, proceed := processTestLine(engine, input.Line{Text: "original", Mode: input.ModeVerbatim})
	if !proceed || effective != (input.Line{Text: "first;second", Mode: input.ModeVerbatim}) {
		t.Fatalf("effective submission = %+v proceed=%v", effective, proceed)
	}
	engine.ExecuteInputLine(effective)

	assertCommands(t, host, []string{"first;second"})
}

func TestOneArgumentInputHookStillObservesVerbatim(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.hooks.on("input", function(text)
			observed = text
		end, { priority = 90 })
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	dispatchTestLine(engine, input.Line{Text: "two", Mode: input.ModeVerbatim})

	assertCommands(t, host, []string{"two"})
	assertLua(t, engine, `assert(observed == "two")`)
}

func TestSendRawSplitsEmbeddedNewlines(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("test", `rune.send_raw("north\nlook\r\nsay hi\rwait\n")`); err != nil {
		t.Fatal(err)
	}
	assertCommands(t, host, []string{"north", "look", "say hi", "wait", ""})
}

// Expansion guards: an alias loop or an oversized repeat stops the whole
// top-level expansion with one error and sends nothing it hasn't already
// sent. Regression: loops used to send their siblings 100 times, x -> x;x
// printed millions of errors, and #N had no limit.
func TestSendExpansionGuards(t *testing.T) {
	cases := []struct {
		name    string
		setup   string
		send    string
		want    []string
		errWant string
	}{
		{
			name:    "self-referencing alias with a sibling sends nothing",
			setup:   `rune.alias.exact("a", "a;look")`,
			send:    "a",
			want:    nil,
			errWant: "Alias loop",
		},
		{
			name:    "branching alias loop stops with one error",
			setup:   `rune.alias.exact("x", "x;x")`,
			send:    "x",
			want:    nil,
			errWant: "Alias loop",
		},
		{
			name:    "repeat over the command budget is rejected before sending",
			send:    "#100000 look",
			want:    nil,
			errWant: "Too many commands",
		},
		{
			name: "repeat at the budget is allowed",
			send: "#1000 look",
			want: func() []string {
				out := make([]string, 1000)
				for i := range out {
					out[i] = "look"
				}
				return out
			}(),
		},
		{
			name:    "alias function that re-sends itself joins the expansion",
			setup:   `rune.alias.exact("r", function() rune.send("r;look") end)`,
			send:    "r",
			want:    nil,
			errWant: "Alias loop",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if tc.setup != "" {
				if err := engine.DoString("setup", tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			host.DrainPrintCalls()
			if err := engine.DoString("send", fmt.Sprintf("rune.send(%q)", tc.send)); err != nil {
				t.Fatal(err)
			}
			sent := host.DrainNetworkCalls()
			if len(sent) != len(tc.want) {
				t.Fatalf("sent %d commands, want %d (first: %v)", len(sent), len(tc.want), head(sent))
			}
			errors := 0
			for _, p := range host.DrainPrintCalls() {
				if strings.Contains(p, "[Error]") {
					errors++
					if !strings.Contains(p, tc.errWant) {
						t.Errorf("error %q does not mention %q", p, tc.errWant)
					}
				}
			}
			wantErrors := 0
			if tc.errWant != "" {
				wantErrors = 1
			}
			if errors != wantErrors {
				t.Errorf("printed %d errors, want %d", errors, wantErrors)
			}
		})
	}
}

// A failed send stops the expansion: the error is echoed once, not once
// per remaining repeat.
func TestSendExpansionStopsAtFirstFailedSend(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.SendErr = errNotConnected
	if err := engine.DoString("send", `rune.send("#5 look;north")`); err != nil {
		t.Fatal(err)
	}
	errors := 0
	for _, p := range host.DrainPrintCalls() {
		if strings.Contains(p, "not connected") {
			errors++
		}
	}
	if errors != 1 {
		t.Errorf("printed %d send errors, want 1", errors)
	}
}

func head(s []string) []string {
	if len(s) > 3 {
		return s[:3]
	}
	return s
}

// A watchdog interruption inside an expansion must not leave the expansion
// marked active, or every later rune.send would silently join a dead one.
func TestSendRecoversAfterRunawayInsideExpansion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CallTimeout = 100 * time.Millisecond
	if err := engine.DoString("setup", `rune.alias.exact("spin", function() `+runawayLoop(engine)+` end)`); err != nil {
		t.Fatal(err)
	}
	if err := engine.DoString("spin", `rune.send("spin")`); err == nil {
		t.Fatal("expected the watchdog to interrupt the runaway alias")
	}
	host.DrainNetworkCalls()
	if err := engine.DoString("after", `rune.send("look")`); err != nil {
		t.Fatal(err)
	}
	if got := host.DrainNetworkCalls(); len(got) != 1 || got[0] != "look" {
		t.Fatalf("sent %q after recovery, want [look]", got)
	}
}
