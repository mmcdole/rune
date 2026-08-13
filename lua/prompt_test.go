package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

func TestPromptHookReportsConfirmation(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		updates = {}
		rune.hooks.on("prompt", function(line, confirmed)
			updates[#updates + 1] = {
				text = line:clean(),
				confirmed = confirmed,
			}
			if not confirmed then return false end
			return line:clean() .. " [styled]"
		end, { priority = 10 })
	`); err != nil {
		t.Fatal(err)
	}

	if got := engine.OnPrompt(text.NewLine("User"), false); got != "" {
		t.Fatalf("gagged partial line = %q", got)
	}
	if got := engine.OnPrompt(text.NewLine("Username:"), true); got != "Username: [styled]" {
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

func TestFlushSpansBeforePromptHooks(t *testing.T) {
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
	engine.FlushSpans()
	engine.OnPrompt(text.NewLine("HP>"), true)
	assertLua(t, engine, `
		assert(#events == 2, "events: " .. #events)
		assert(events[1] == "span" and events[2] == "hook",
			"boundary ordering: " .. table.concat(events, ","))
	`)
}

func TestPromptTriggerDispatch(t *testing.T) {
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

	engine.OnPrompt(text.NewLine("Username:"), false)
	engine.OnPrompt(text.NewLine("Username:"), true)
	engine.OnOutput(text.NewLine("Username:"))

	assertLua(t, engine, `
		assert(output_fired == 1, "default output trigger fired: " .. output_fired)
		assert(#prompt_flags == 2, "prompt trigger fired: " .. #prompt_flags)
		assert(prompt_flags[1] == false, "partial flag")
		assert(prompt_flags[2] == true, "confirmed flag")
	`)
}

func TestPromptTriggerOptionsValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want string
	}{
		{name: "unknown channel", code: `rune.trigger.exact("x", nil, { on = "both" })`, want: `on must be "output" or "prompt"`},
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
}
