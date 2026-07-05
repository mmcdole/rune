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
