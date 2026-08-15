package lua

// Tests for the single-identity rule (15_registry.lua): a registry entry
// with a natural key (a bind's key, a bar's layout name, a command's name,
// an exact alias's phrase) is registered under that key as its name, so
// the whole management suite addresses it by the string the user typed.
// Anything that registers under two identities is the bug these cover.

import (
	"strings"
	"testing"
)

// Registrations made without opts still answer to the management suite,
// which is what the two-identity split used to prevent.
func TestNaturalKeyIsTheName(t *testing.T) {
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
		assert(rune.binds.get("pageup"), "a default keymap bind must be addressable")
		assert(rune.command.get("quit"), "a built-in command must be addressable")
	`)
}

// One entry, one identity: a legacy name is dropped with a notice. It
// must not raise, because raising would abort the rest of the script
// that carried it, costing every registration below the stale line.
func TestKeyedRegistriesDeprecateAnExplicitName(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		key     string
		present string // a management call proving the entry registered
	}{
		{
			name:    "bind",
			code:    `rune.bind("f1", function() end, { name = "other" })`,
			key:     "f1",
			present: `assert(rune.binds.get("f1"), "the bind must still register")`,
		},
		{
			name:    "bar",
			code:    `rune.ui.bar("clock", function() return "" end, { name = "other" })`,
			key:     "clock",
			present: `assert(rune.bars.get("clock"), "the bar must still register")`,
		},
		{
			name:    "exact alias",
			code:    `rune.alias.exact("gc", "get corpse", { name = "other" })`,
			key:     "gc",
			present: `assert(rune.alias.get("gc"), "the alias must still register")`,
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

// The whole point of not raising: everything after the stale line loads.
func TestALegacyNameDoesNotAbortTheScript(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("init.lua", `
		rune.alias.exact("before", "loaded first")
		rune.bind("f5", function() end, { name = "legacy-name" })
		rune.alias.exact("after", "loaded after the stale option")
	`); err != nil {
		t.Fatalf("the script must survive a legacy name: %v", err)
	}

	assertLua(t, engine, `
		assert(rune.alias.get("before"), "registrations before the stale line must survive")
		assert(rune.alias.get("after"), "registrations after the stale line must survive")
		assert(rune.binds.get("f5"), "the bind itself must register under its key")
	`)
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
		local scroll = assert(rune.binds.get("pageup")):action()
		rune.bind("pageup", function()
			scroll()
			wrapped = true
		end)
	`)

	assertLua(t, engine, `
		assert(rune.binds._dispatch("pageup"), "pageup should still be bound")
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
