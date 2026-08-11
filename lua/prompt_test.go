package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

func TestPromptHookCarriesExplicitConfirmation(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		updates = {}
		rune.hooks.on("prompt", function(line, confirmed)
			updates[#updates + 1] = {
				text = line:clean(),
				confirmed = confirmed,
			}
			return line:clean() .. " [styled]"
		end, { priority = 10 })
	`); err != nil {
		t.Fatal(err)
	}

	if got := engine.OnPartial(text.NewLine("User")); got != "User [styled]" {
		t.Fatalf("partial rewrite = %q", got)
	}
	if got := engine.OnPrompt(text.NewLine("Username:")); got != "Username: [styled]" {
		t.Fatalf("confirmed rewrite = %q", got)
	}

	assertLua(t, engine, `
		assert(#updates == 2, "updates: " .. #updates)
		assert(updates[1].text == "User" and updates[1].confirmed == false,
			"first update must be unconfirmed")
		assert(updates[2].text == "Username:" and updates[2].confirmed == true,
			"second update must be confirmed")
	`)
}

func TestConfirmedPromptFlushesSpansBeforePromptHooks(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		events = {}
		rune.trigger.starts("Story:", function()
			events[#events + 1] = "span"
		end, { span = { to = "NEVER", max = 8 } })
		rune.hooks.on("prompt", function()
			events[#events + 1] = "hook"
		end, { priority = 1 })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("Story: unfinished"))
	engine.OnPrompt(text.NewLine("HP>"))
	assertLua(t, engine, `
		assert(#events == 2, "events: " .. #events)
		assert(events[1] == "span" and events[2] == "hook",
			"boundary ordering: " .. table.concat(events, ","))
	`)
}

func TestPromptHookCanGagOverlayText(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.hooks.on("prompt", function(line)
			if line:clean() == "Password:" then return false end
		end, { priority = 10 })
	`); err != nil {
		t.Fatal(err)
	}

	if got := engine.OnPartial(text.NewLine("Password:")); got != "" {
		t.Fatalf("gagged prompt update displayed as %q", got)
	}
}

func TestTriggerChannelsAreExclusive(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		output_fired = 0
		prompt_flags = {}
		rune.trigger.exact("Username:", function(matches, ctx)
			output_fired = output_fired + 1
			assert(ctx.confirmed == nil)
		end)
		rune.trigger.exact("Username:", function(matches, ctx)
			prompt_flags[#prompt_flags + 1] = ctx.confirmed
		end, { on = "prompt" })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnPartial(text.NewLine("Username:"))
	engine.OnPrompt(text.NewLine("Username:"))
	engine.OnOutput(text.NewLine("Username:"))

	assertLua(t, engine, `
		assert(output_fired == 1, "default output trigger fired: " .. output_fired)
		assert(#prompt_flags == 2, "prompt trigger fired: " .. #prompt_flags)
		assert(prompt_flags[1] == false, "partial flag")
		assert(prompt_flags[2] == true, "confirmed flag")
	`)
}

func TestConfirmedOnlyPromptTrigger(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.starts("HP:", function(matches, ctx)
			fired = fired + 1
			assert(ctx.confirmed == true)
		end, { on = "prompt", confirmed_only = true })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnPartial(text.NewLine("HP: 10"))
	engine.OnPrompt(text.NewLine("HP: 100>"))
	assertLua(t, engine, `assert(fired == 1, "fired: " .. fired)`)
}

func TestOncePromptTriggerSurvivesCumulativeUpdatesOnlyOnce(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.starts("User", function()
			fired = fired + 1
		end, { on = "prompt", once = true })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnPartial(text.NewLine("User"))
	engine.OnPartial(text.NewLine("Username:"))
	engine.OnPrompt(text.NewLine("Username:"))
	assertLua(t, engine, `assert(fired == 1, "once trigger fired: " .. fired)`)
}

func TestPromptTriggerOptionsValidateAndList(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want string
	}{
		{name: "unknown channel", code: `rune.trigger.exact("x", nil, { on = "both" })`, want: `on must be "output" or "prompt"`},
		{name: "channel type", code: `rune.trigger.exact("x", nil, { on = true })`, want: `on must be "output" or "prompt"`},
		{name: "false channel", code: `rune.trigger.exact("x", nil, { on = false })`, want: `on must be "output" or "prompt"`},
		{name: "confirmed type", code: `rune.trigger.exact("x", nil, { on = "prompt", confirmed_only = "yes" })`, want: "confirmed_only must be a boolean"},
		{name: "confirmed output", code: `rune.trigger.exact("x", nil, { confirmed_only = true })`, want: `confirmed_only requires on = "prompt"`},
		{name: "prompt span", code: `rune.trigger.exact("x", nil, { on = "prompt", span = { max = 2 } })`, want: `span requires on = "output"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _, cleanup := setupTest(t)
			defer cleanup()
			err := engine.DoString("invalid option", tc.code)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	engine, _, cleanup := setupTest(t)
	defer cleanup()
	if err := engine.DoString("list", `
		rune.trigger.exact("line", nil)
		rune.trigger.exact("HP:", nil, { on = "prompt", confirmed_only = true })
		local list = rune.trigger.list()
		assert(list[1].on == "output" and list[1].confirmed_only == false)
		assert(list[2].on == "prompt" and list[2].confirmed_only == true)
	`); err != nil {
		t.Fatal(err)
	}
}

func TestPartialProjectsOutputGagWithoutFiringTrigger(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = 0
		rune.trigger.exact("SECRET", function()
			fired = fired + 1
		end, { gag = true })
	`); err != nil {
		t.Fatal(err)
	}

	if got := engine.OnPartial(text.NewLine("SECRET")); got != "" {
		t.Fatalf("gagged output match flashed in partial overlay as %q", got)
	}
	assertLua(t, engine, `assert(fired == 0, "partial fired output trigger")`)

	// GA/EOR makes this a prompt record, not completed output. Output-channel
	// gag projection is intentionally provisional; prompt-channel policy owns
	// the confirmed record.
	if got := engine.OnPrompt(text.NewLine("SECRET")); got != "SECRET" {
		t.Fatalf("confirmed prompt inherited output-only gag as %q", got)
	}
	assertLua(t, engine, `assert(fired == 0, "confirmed prompt fired output trigger")`)

	if _, show := engine.OnOutput(text.NewLine("SECRET")); show {
		t.Fatal("completed output match should be gagged")
	}
	assertLua(t, engine, `assert(fired == 1, "completed output did not fire trigger")`)
}
