package lua

// Smoke tests for the listing slash commands (55_commands.lua): each
// must run without error and mention what was registered. These guard
// the /listing surface that has no other coverage - a formatting typo
// in a listing otherwise only surfaces when a user types it.

import (
	"strings"
	"testing"
)

func TestListingCommandsShowRegistrations(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.alias.exact("zap", "cast zap", { name = "my-alias" })
		rune.trigger.contains("dragon", "flee", { name = "my-trigger" })
		rune.timer.every(60, function() end, { name = "my-timer" })
		rune.hooks.on("output", function() end, { name = "my-hook" })
		rune.bind("f12", function() end, { name = "my-bind" })
		rune.ui.bar("my-bar", function() return "bar" end)
		rune.group.disable("my-group")
	`); err != nil {
		t.Fatal(err)
	}
	host.DrainPrintCalls()

	cases := []struct {
		command string
		want    string // substring that must appear in the listing
	}{
		{"/aliases", "zap"},
		{"/triggers", "dragon"},
		{"/timers", "my-timer"},
		{"/hooks", "my-hook"},
		{"/binds", "f12"},
		{"/bars", "my-bar"},
		{"/groups", "my-group"},
		{"/help", "/connect"},
		{"/version", "Rune"},
	}

	for _, c := range cases {
		engine.OnInput(c.command)
		printed := strings.Join(host.DrainPrintCalls(), "\n")

		if printed == "" {
			t.Errorf("%s printed nothing", c.command)
			continue
		}
		if strings.Contains(printed, "[Error]") || strings.Contains(printed, "error:") {
			t.Errorf("%s reported an error:\n%s", c.command, printed)
		}
		if !strings.Contains(printed, c.want) {
			t.Errorf("%s listing missing %q:\n%s", c.command, c.want, printed)
		}
	}

	// Listing commands must never reach the server.
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("listing commands leaked to the network: %v", sent)
	}
}

// Invalid user metadata must fail at registration, before it can poison
// the command registry consumed by /help and the inline command picker.
func TestCommandRegistrationRejectsTableDescriptionWithoutBreakingHelp(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("user.lua", `
		rune.command.add("autoreconnect", function() end, {
			name = "automatic-reconnect-command",
			group = "connection",
		})
	`)
	if err == nil {
		t.Error("registration with a table description succeeded")
	} else if !strings.Contains(err.Error(), "description must be a string") {
		t.Errorf("registration error = %q, want description type error", err)
	}

	host.DrainPrintCalls()
	engine.OnInput("/help")
	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if strings.Contains(printed, "error:") {
		t.Errorf("/help was broken by the rejected command:\n%s", printed)
	}
	if !strings.Contains(printed, "/connect") {
		t.Errorf("/help did not list built-in commands:\n%s", printed)
	}
	if strings.Contains(printed, "/autoreconnect") {
		t.Errorf("rejected command remained registered:\n%s", printed)
	}
}

func TestCommandRegistrationRejectsInvalidName(t *testing.T) {
	cases := []struct {
		name   string
		setup  string
		assert string
	}{
		{
			name:   "empty",
			setup:  `rune.command.add("", function() end)`,
			assert: `assert(rune.command.get("") == nil)`,
		},
		{
			name:   "multiple words",
			setup:  `rune.command.add("chat off", function() end)`,
			assert: `assert(rune.command.get("chat off") == nil)`,
		},
		{
			name:   "tab separated",
			setup:  `rune.command.add("chat\toff", function() end)`,
			assert: `assert(rune.command.get("chat\toff") == nil)`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, _, cleanup := setupTest(t)
			defer cleanup()

			err := engine.DoString("user.lua", tc.setup)
			if err == nil {
				t.Fatal("invalid command name was accepted")
			}
			if !strings.Contains(err.Error(), "name must be a non-empty single word") {
				t.Fatalf("registration error = %q, want command name error", err)
			}
			if err := engine.DoString("assert.lua", tc.assert); err != nil {
				t.Fatalf("rejected command remained registered: %v", err)
			}
		})
	}
}

// TestErrorTagIsRed pins the presentation convention (05_style.lua):
// [Error] tags are red, tag only, message plain - checked on the two
// highest-traffic paths, the default error handler and unknown
// commands.
func TestErrorTagIsRed(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.NotifyError("something broke")
	engine.OnInput("/nosuchcommand")

	printed := strings.Join(host.DrainPrintCalls(), "\n")
	for _, want := range []string{
		"\x1b[31m[Error]\x1b[0m something broke",
		"\x1b[31m[Error]\x1b[0m Unknown command: /nosuchcommand",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("missing red-tagged error %q in:\n%s", want, printed)
		}
	}
}
