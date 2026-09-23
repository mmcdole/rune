package lua

// Tests for registry addresses (20_registry.lua): an entry is reachable
// by its natural key (a bind's key, an exact alias's phrase, a bar's or
// command's name) and by its explicit name, and each address identifies
// at most one entry. Two entries answering to one string, or one string
// silently changing which entry it means, is the bug these cover.

import (
	"strings"
	"testing"
)

// Registrations made without opts answer to their key, and report it
// as their name since they were given no other.
func TestKeyAddressesAnUnnamedEntry(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	cases := []struct {
		name  string
		setup string
		check string
	}{
		{
			name:  "bind",
			setup: `rune.bind("ctrl+g", function() end)`,
			check: `
				assert(rune.binds.get("ctrl+g"), "bind not addressable by key")
				assert(rune.binds.get("ctrl+g"):name() == "ctrl+g", "key must be the name")
				assert(rune.binds.disable("ctrl+g"), "disable by key must report success")
			`,
		},
		{
			name:  "bar",
			setup: `rune.ui.bar("vitals", function() return "" end)`,
			check: `
				assert(rune.bars.get("vitals"), "bar not addressable by layout name")
				assert(rune.bars.disable("vitals"), "disable by bar name must report success")
			`,
		},
		{
			name:  "exact alias",
			setup: `rune.alias.exact("gc", "get all from corpse")`,
			check: `
				assert(rune.alias.get("gc"), "exact alias not addressable by phrase")
				assert(rune.alias.disable("gc"), "disable by phrase must report success")
			`,
		},
		{
			name:  "multi-word exact alias uses its normalized phrase",
			setup: `rune.alias.exact("chat   off", "chatlog off")`,
			check: `assert(rune.alias.get("chat off"), "phrase must be normalized before naming")`,
		},
		{
			name:  "command",
			setup: `rune.command.add("greet", function() end, "Greet")`,
			check: `
				assert(rune.command.get("greet"), "command not addressable by name")
				assert(rune.command.disable("greet"), "disable by command name must report success")
			`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := engine.DoString("setup", c.setup); err != nil {
				t.Fatal(err)
			}
			assertLua(t, engine, c.check)
		})
	}
}

// The core's own registrations are made without opts, so they are the
// real regression target: before the single-identity rule they were
// unreachable by name.
func TestCoreRegistrationsAreAddressable(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		assert(rune.bars.get("status"), "the core status bar must be addressable")
		assert(rune.bars.disable("status"), "the core status bar must be manageable")
		assert(rune.bars.toggle("status"), "the core status bar must be toggleable")
		assert(rune.bars.toggle("status"), "the core status bar must toggle back")
		assert(rune.bars.toggle("missing") == false, "an unknown bar must not be created")
		assert(rune.binds.get("pgup"), "a default keymap bind must be addressable")
		assert(rune.command.get("quit"), "a built-in command must be addressable")
	`)
}

// Bars and commands take no explicit name: one is dropped with a notice.
// It must not raise, because raising would abort the rest of the script
// that carried it, costing every registration below the stale line.
func TestBarsAndCommandsDropAnExplicitName(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		key     string
		present string // a management call proving the entry registered
	}{
		{
			name:    "bar",
			code:    `rune.ui.bar("clock", function() return "" end, { name = "other" })`,
			key:     "clock",
			present: `assert(rune.bars.get("clock"), "the bar must still register")`,
		},
		{
			name:    "command",
			code:    `rune.command.add("greet", function() end, "d", { name = "other" })`,
			key:     "greet",
			present: `assert(rune.command.get("greet"), "the command must still register")`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			host.DrainPrintCalls()

			if err := engine.DoString("legacy", c.code); err != nil {
				t.Fatalf("a legacy name must not raise: %v", err)
			}

			printed := strings.Join(host.DrainPrintCalls(), "\n")
			if !strings.Contains(printed, "[Deprecated]") {
				t.Fatalf("expected a deprecation notice, got: %q", printed)
			}
			if !strings.Contains(printed, c.key) {
				t.Fatalf("the notice must name the key to use, got: %q", printed)
			}

			assertLua(t, engine, c.present)
		})
	}
}

// A script that never used the option must stay quiet, and a name that
// simply repeats the key is not a migration problem either.
func TestNoNoticeWithoutALegacyName(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.DrainPrintCalls()

	if err := engine.DoString("quiet", `
		rune.bind("f3", function() end)
		rune.bind("f4", function() end, { group = "combat" })
		rune.bind("f6", function() end, { name = "f6" })
	`); err != nil {
		t.Fatal(err)
	}

	if printed := strings.Join(host.DrainPrintCalls(), "\n"); printed != "" {
		t.Fatalf("expected no notice, got: %q", printed)
	}
}

// A name is a second address for the same entry. Adding one changes
// nothing about addressing the entry by its key.
func TestANameIsASecondAddress(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		local heal = rune.bind("f1", function() end, { name = "combat.heal" })
		assert(rune.binds.get("f1") == heal, "the key must address the named bind")
		assert(rune.binds.get("combat.heal") == heal, "the name must address the bind")
		assert(heal:name() == "combat.heal", "the handle must report the explicit name")
		assert(rune.binds.disable("f1"), "disable by key")
		assert(rune.binds.enable("combat.heal"), "enable by name")
		assert(rune.unbind("combat.heal"), "unbind by name")
		assert(rune.binds.get("f1") == nil, "unbinding by name must release the key")

		local loot = rune.alias.exact("get   corpse", "get all from corpse", { name = "loot.corpse" })
		assert(rune.alias.get("get corpse") == loot, "the normalized phrase must address the alias")
		assert(rune.alias.get("loot.corpse") == loot, "the name must address the alias")
		assert(rune.alias.list()[1].name == "loot.corpse", "listings must carry the name")
	`)
}

// Replacing through one address releases the entry's other address.
func TestReplacementReleasesTheOtherAddress(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		rune.bind("f1", function() end, { name = "combat.heal" })
		local moved = rune.bind("f2", function() end, { name = "combat.heal" })
		assert(rune.binds.get("f1") == nil, "moving a named bind must release its old key")
		assert(rune.binds.get("f2") == moved and rune.binds.get("combat.heal") == moved)

		local rebound = rune.bind("f2", function() end)
		assert(rune.binds.get("combat.heal") == nil, "rebinding a key must release the old name")
		assert(rune.binds.get("f2") == rebound and rebound:name() == "f2")

		-- Across kinds, a name replaces like any other name.
		rune.alias.exact("gc", "get corpse", { name = "loot" })
		rune.alias.regex("^lc$", "look corpse", { name = "loot" })
		assert(rune.alias.get("gc") == nil, "the exact alias must be gone")
		assert(rune.alias.get("loot"):action() == "look corpse")
	`)
	if err := engine.DoString("dispatch", `rune.send("gc")`); err != nil {
		t.Fatal(err)
	}
	assertLua(t, engine, `assert(rune.alias.count() == 1, "the replaced phrase must not linger in dispatch")`)
}

// A registration that would take an address across the key/name line, or
// replace two entries at once, is refused before anything changes.
func TestConflictingRegistrationsAreRefused(t *testing.T) {
	cases := []struct {
		name  string
		setup string
		code  string
		want  string // fragment of the error
	}{
		{
			name:  "name spells another entry's phrase",
			setup: `rune.alias.exact("heal", "drink potion")`,
			code:  `rune.alias.exact("h", "cast heal", { name = "heal" })`,
			want:  `name 'heal' is the key of alias "heal"`,
		},
		{
			name:  "phrase spells another entry's name",
			setup: `rune.alias.exact("h", "cast heal", { name = "heal" })`,
			code:  `rune.alias.exact("heal", "drink potion")`,
			want:  `key 'heal' is the name of alias "heal" on "h"`,
		},
		{
			name:  "regex alias named after a phrase",
			setup: `rune.alias.exact("gc", "get corpse")`,
			code:  `rune.alias.regex("^gc$", "get corpse", { name = "gc" })`,
			want:  `name 'gc' is the key of alias "gc"`,
		},
		{
			name:  "key spells another bind's name",
			setup: `rune.bind("f1", function() end, { name = "f2" })`,
			code:  `rune.bind("f2", function() end)`,
			want:  `key 'f2' is the name of bind "f2" on "f1"`,
		},
		{
			name: "one registration would replace two binds",
			setup: `
				rune.bind("f1", function() end, { name = "combat.heal" })
				rune.bind("f2", function() end, { name = "combat.flee" })
			`,
			code: `rune.bind("f2", function() end, { name = "combat.heal" })`,
			want: `would replace both bind "combat.flee" on "f2"`,
		},
		{
			name:  "a later array element conflicts",
			setup: `rune.bind("f1", function() end, { name = "f8" })`,
			code:  `rune.bind({"f7", "f8"}, function() end)`,
			want:  `key 'f8' is the name of`,
		},
		{
			name: "a name on an array of keys",
			code: `rune.bind({"f7", "f8"}, function() end, { name = "x" })`,
			want: `a name addresses one bind`,
		},
		{
			name: "an empty name",
			code: `rune.alias.exact("gc", "get corpse", { name = "" })`,
			want: `name must be a non-empty string`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			engine, _, cleanup := setupTest(t)
			defer cleanup()

			if err := engine.DoString("setup", `
				`+c.setup+`
				before = { binds = rune.binds.list(), aliases = rune.alias.list() }
			`); err != nil {
				t.Fatal(err)
			}

			err := engine.DoString("conflict", c.code)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want %q", err, c.want)
			}
			if !strings.Contains(err.Error(), `"conflict"]:1:`) {
				t.Fatalf("the error must point at the registration, got %v", err)
			}

			assertLua(t, engine, `
				local function same(a, b)
					assert(#a == #b, "a refused registration must not change the registry")
					for i, item in ipairs(a) do
						assert(item.key == b[i].key and item.match == b[i].match and
							item.name == b[i].name and item.enabled == b[i].enabled)
					end
				end
				same(before.binds, rune.binds.list())
				same(before.aliases, rune.alias.list())
			`)
		})
	}
}

// Callbacks and listings see the effective name.
func TestAliasContextReportsTheName(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		rune.alias.exact("h", function(args, ctx) rune.send_raw(ctx.name) end, { name = "heal" })
		rune.alias.exact("gc", function(args, ctx) rune.send_raw(ctx.name) end)
	`)
	dispatchTestCommand(engine, "h")
	dispatchTestCommand(engine, "gc")
	if sent := host.DrainNetworkCalls(); len(sent) != 2 || sent[0] != "heal" || sent[1] != "gc" {
		t.Fatalf("ctx.name = %v, want the name then the phrase", sent)
	}

	host.DrainPrintCalls()
	assertLua(t, engine, `rune.bind("f1", function() end, { name = "combat.heal" })`)
	dispatchTestCommand(engine, "/binds")
	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if !strings.Contains(printed, "combat.heal") {
		t.Fatalf("/binds must show a name that differs from the key, got: %q", printed)
	}
}

// Re-registering a natural key replaces rather than accumulates, and the
// replacement is what you get back. The old handle going away must not
// take the live entry's index slot with it.
func TestReRegisteringAKeyReplaces(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		local count_before = rune.binds.count()
		local first = rune.bind("ctrl+g", function() end)
		local second = rune.bind("ctrl+g", function() end)

		assert(rune.binds.get("ctrl+g") == second, "get must return the replacement")
		assert(rune.binds.get("ctrl+g") ~= first, "the old handle must not linger")
		assert(rune.binds.count() == count_before + 1, "replacing must not grow the registry")

		-- Removing the displaced handle is a no-op that must not evict the
		-- live binding from the dispatch index.
		first:remove()
		assert(rune.binds.get("ctrl+g") == second, "removing the old handle must not evict the new one")
	`)
}

// h:action() returns the registered action across registries, which is
// the supported way to wrap an existing callback.
func TestHandleActionReturnsTheRegisteredAction(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		local fn = function() end
		rune.bind("f2", fn)
		assert(rune.binds.get("f2"):action() == fn, "bind action must be the callback")

		local render = function() return "" end
		rune.ui.bar("clock", render)
		assert(rune.bars.get("clock"):action() == render, "bar action must be the renderer")

		local handler = function() end
		rune.command.add("greet", handler, "Greet")
		assert(rune.command.get("greet"):action() == handler, "command action must be the handler")

		-- A string action comes back as the string, not wrapped in a function.
		rune.trigger.contains("hungry", "eat bread", { name = "feeder" })
		assert(rune.trigger.get("feeder"):action() == "eat bread", "string actions must round-trip")

		local on_out = function() end
		rune.hooks.on("output", on_out, { name = "watcher" })
		assert(rune.hooks.get("watcher"):action() == on_out, "hook action must be the handler")
	`)
}

// The documented pattern for extending a built-in: capture the action,
// re-register the key, call through.
func TestWrappingABuiltInCommand(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		local original = assert(rune.command.get("echo")):action()
		rune.command.add("echo", function(args)
			original("wrapped: " .. args)
		end, "Echo, wrapped")
	`); err != nil {
		t.Fatal(err)
	}

	dispatchTestCommand(engine, "/echo hello")

	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if !strings.Contains(printed, "wrapped: hello") {
		t.Fatalf("wrapper did not call through, got: %q", printed)
	}
}

// The same pattern for a default bind, verbatim as the guides print it.
func TestWrappingADefaultBind(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		wrapped = false
		local scroll = assert(rune.binds.get("pgup")):action()
		rune.bind("pgup", function()
			scroll()
			wrapped = true
		end)
	`)

	assertLua(t, engine, `
		assert(rune.binds._dispatch("pgup"), "pgup should still be bound")
		assert(wrapped, "the wrapper did not run")
	`)
}

// get() addresses only what is registered, and unbinding takes the key
// out of the index it was registered under.
func TestGetReturnsNilForUnknownAndRemoved(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	assertLua(t, engine, `
		assert(rune.binds.get("f9") == nil, "unknown key must be nil")
		assert(rune.trigger.get("nope") == nil, "unknown trigger name must be nil")

		rune.bind("f9", function() end)
		assert(rune.binds.get("f9"), "bind should be present after binding")
		assert(rune.unbind("f9"), "unbind must report success")
		assert(rune.binds.get("f9") == nil, "unbound key must be gone")
		assert(rune.unbind("f9") == false, "unbinding twice must report failure")
	`)
}
