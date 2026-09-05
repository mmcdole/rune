package lua

// Command expansion semantics (75_send.lua): the variant matrix for
// semicolon splitting and #N repeats. The e2e wiring proof lives in
// test/e2e/scenarios/send.json.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
)

func TestSendExpansion(t *testing.T) {
	runFeatureCases(t, []featureCase{
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

func TestRepeatBlocksReportMigrationErrorWithoutSendingFragments(t *testing.T) {
	for _, text := range []string{
		"#2 {kill rat;loot}",
		"look;#2 {kill rat;loot};west",
		"#2 {kill rat;loot",
		"#2{north}",
		"#2 {}",
	} {
		t.Run(text, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			assertLua(t, engine, fmt.Sprintf(`rune.send(%q)`, text))
			assertCommands(t, host, nil)
			if output := strings.Join(host.DrainPrintCalls(), "\n"); !strings.Contains(output, "repeat an alias with #N name instead") {
				t.Fatalf("missing migration guidance: %q", output)
			}
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

func TestVerbatimInputPreservesLinesAndBypassesCommands(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.alias.exact("aliased", "expanded")
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	draft := "  indented;still one line;;  \n\n/quit\n#2 north\naliased\ntrailing  \n"
	dispatchTestSubmission(engine, input.Verbatim(draft))

	assertCommands(t, host, []string{
		"  indented;still one line;;  ",
		"",
		"/quit",
		"#2 north",
		"aliased",
		"trailing  ",
		"",
	})
	if host.QuitCalled {
		t.Fatal("verbatim /quit must be sent as data")
	}
}

func TestVerbatimInputDegradedModePreservesPhysicalLines(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("sabotage", "rune.hooks = nil"); err != nil {
		t.Fatalf("sabotage failed: %v", err)
	}

	dispatchTestSubmission(engine, input.Verbatim("first\r\n\r/quit\r"))

	assertCommands(t, host, []string{"first", "", "/quit", ""})
	if host.QuitCalled {
		t.Fatal("degraded verbatim /quit must be sent as data")
	}
}

func TestInputWithCommandContextKeepsNormalExpansion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	dispatchTestSubmission(engine, input.Command("look;#2 north"))

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
		rune.input._dispatch("look;#2 north", "command")
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
		rune.input._dispatch("first;second\n/quit", "verbatim")
	`); err != nil {
		t.Fatal(err)
	}

	assertCommands(t, host, []string{"first;second", "/quit"})
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

	draft := "first;second\n/quit"
	dispatchTestSubmission(engine, input.Verbatim(draft))

	assertCommands(t, host, nil)
	assertLua(t, engine, `
		assert(observed_mode == "verbatim")
		assert(observed_text == "first;second\n/quit")
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

	dispatchTestSubmission(engine, input.Verbatim("first;second\n/quit"))

	assertCommands(t, host, []string{"context-readonly", "first;second", "/quit"})
	if host.QuitCalled {
		t.Fatal("mutating one hook context changed canonical verbatim routing")
	}
}

func TestInputRewritePreservesVerbatimMode(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("rewrite verbatim", `
		rune.hooks.on("input", function()
			return "first;second\nthird"
		end, { priority = 90 })
	`); err != nil {
		t.Fatal(err)
	}

	effective, proceed := engine.ApplyInputHooks(input.Verbatim("original"))
	if !proceed || effective != input.Verbatim("first;second\nthird") {
		t.Fatalf("effective submission = %+v proceed=%v", effective, proceed)
	}
	engine.DispatchSubmission(effective)

	assertCommands(t, host, []string{"first;second", "third"})
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

	dispatchTestSubmission(engine, input.Verbatim("one\ntwo"))

	assertCommands(t, host, []string{"one", "two"})
	assertLua(t, engine, `assert(observed == "one\ntwo")`)
}

func TestSendRawSplitsEmbeddedNewlines(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("test", `rune.send_raw("north\nlook\r\nsay hi\rwait\n")`); err != nil {
		t.Fatal(err)
	}
	assertCommands(t, host, []string{"north", "look", "say hi", "wait", ""})
}
