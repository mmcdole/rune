package lua

// Command expansion semantics (75_send.lua): the variant matrix for
// semicolon splitting and #N repeats. The e2e wiring proof lives in
// test/e2e/scenarios/send.json.

import "testing"

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
			input: ";say hello;;look;",
			want:  []string{"", "say hello", "", "look", ""},
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
			name:  "repeat after delimiter",
			input: "open gate;#2 south",
			want:  []string{"open gate", "south", "south"},
		},
		{
			name:  "repeat braced group",
			input: "#2 {kill rat;loot}",
			want:  []string{"kill rat", "loot", "kill rat", "loot"},
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

func TestVerbatimInputPreservesLinesAndBypassesCommands(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.alias.exact("aliased", "expanded")
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	input := "  indented;still one line  \n\n/quit\n#2 north\naliased\ntrailing  \n"
	engine.OnInputWithContext(input, InputContext{Mode: InputModeVerbatim})

	assertCommands(t, host, []string{
		"  indented;still one line  ",
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

func TestVerbatimInputSplitsOnlyOnLF(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnInputWithContext("one\r\ntwo\rthree", InputContext{Mode: InputModeVerbatim})

	assertCommands(t, host, []string{"one\r", "two\rthree"})
}

func TestVerbatimInputDegradedModePreservesEmptyLines(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("sabotage", "rune.hooks = nil"); err != nil {
		t.Fatalf("sabotage failed: %v", err)
	}

	engine.OnInputWithContext("first\n\n/quit\n", InputContext{Mode: InputModeVerbatim})

	assertCommands(t, host, []string{"first", "", "/quit", ""})
	if host.QuitCalled {
		t.Fatal("degraded verbatim /quit must be sent as data")
	}
}

func TestInputWithCommandContextKeepsNormalExpansion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnInputWithContext("look;#2 north", InputContext{Mode: InputModeCommand})

	assertCommands(t, host, []string{"look", "north", "north"})
}

func TestVerbatimInputHookReceivesContextAndCanConsume(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune._input.send_verbatim = nil
		rune.hooks.on("input", function(text, context)
			rune.send_raw(context.mode .. "|" .. text)
			return false
		end, { priority = 90 })
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	input := "first;second\n/quit"
	engine.OnInputWithContext(input, InputContext{Mode: InputModeVerbatim})

	// The observer receives the complete submission once. Returning false
	// prevents the core verbatim sender from emitting either physical line.
	assertCommands(t, host, []string{"verbatim|" + input})
}

func TestVerbatimCoreSenderCannotBeClobberedThroughExport(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `rune._input.send_verbatim = nil`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	engine.OnInputWithContext("one\ntwo", InputContext{Mode: InputModeVerbatim})

	assertCommands(t, host, []string{"one", "two"})
}

func TestOneArgumentInputHookStillObservesVerbatim(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.hooks.on("input", function(text)
			rune.send_raw("observed:" .. text)
		end, { priority = 90 })
	`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	engine.OnInputWithContext("one\ntwo", InputContext{Mode: InputModeVerbatim})

	assertCommands(t, host, []string{"observed:one\ntwo", "one", "two"})
}
