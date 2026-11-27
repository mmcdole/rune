package lua

// Tests for the shared registry factory (registry.lua), exercised
// through the public trigger/alias/hook APIs the way user scripts use
// it: upsert-by-name, priority ordering, once, and mutation during
// dispatch (the snapshot guarantee).

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// TestRegistrationsRecordSource verifies that hooks, triggers, aliases,
// and timers record the registering script's file:line for listings and
// error attribution.
func TestRegistrationsRecordSource(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	script := `
		rune.hooks.on("output", function() end, {name = "h"})
		rune.trigger.contains("x", "look", {name = "t"})
		rune.alias.exact("n", "north")
		rune.timer.after(60, function() end, {name = "tm"})

		local function source_of(list, name)
			for _, item in ipairs(list) do
				if item.name == name then
					return item.source
				end
			end
		end

		for _, entry in ipairs({
			{"hook", rune.hooks.list(), "h"},
			{"trigger", rune.trigger.list(), "t"},
			{"alias", rune.alias.list(), "n"},
			{"timer", rune.timer.list(), "tm"},
		}) do
			local src = source_of(entry[2], entry[3])
			assert(src and src:find("attr_test"),
				entry[1] .. " source not recorded, got " .. tostring(src))
		end
	`
	if err := engine.DoString("attr_test.lua", script); err != nil {
		t.Fatalf("source attribution failed: %v", err)
	}
}

func TestRegistryUpsertByNameReplaces(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.trigger.contains("food", "eat old", { name = "eater" })
		-- Same name, even disabled first: the new registration replaces
		-- the old wholesale and starts enabled.
		rune.trigger.disable("eater")
		rune.trigger.contains("food", "eat new", { name = "eater" })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("you see food here"))

	sent := host.DrainNetworkCalls()
	if len(sent) != 1 || sent[0] != "eat new" {
		t.Fatalf("expected exactly the replacement action, got %v", sent)
	}
	assertLua(t, engine, `assert(#rune.trigger.list() == 1, "upsert must not grow the registry")`)
}

func TestRegistryPriorityOrderAndInsertionTiebreak(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = {}
		local function mark(label)
			return function() fired[#fired + 1] = label end
		end
		-- Registered out of order on purpose; equal priorities keep
		-- insertion order.
		rune.hooks.on("output", mark("p30"),   { priority = 30 })
		rune.hooks.on("output", mark("p10-a"), { priority = 10 })
		rune.hooks.on("output", mark("p20"),   { priority = 20 })
		rune.hooks.on("output", mark("p10-b"), { priority = 10 })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("anything"))

	assertLua(t, engine, `
		local got = table.concat(fired, ",")
		assert(got == "p10-a,p10-b,p20,p30", "dispatch order was: " .. got)
	`)
}

func TestRegistryOnceRemovesAfterFirstFire(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.trigger.contains("gong", "answer gong", { once = true })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("the gong sounds"))
	engine.OnOutput(text.NewLine("the gong sounds again"))

	sent := host.DrainNetworkCalls()
	if len(sent) != 1 {
		t.Fatalf("once trigger fired %d times: %v", len(sent), sent)
	}
	assertLua(t, engine, `assert(#rune.trigger.list() == 0, "once trigger should be removed")`)
}

func TestRegistryOnceAlias(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		rune.alias.exact("gg", "cast gate", { once = true })
	`); err != nil {
		t.Fatal(err)
	}

	dispatchTestCommand(engine, "gg")
	dispatchTestCommand(engine, "gg")

	sent := host.DrainNetworkCalls()
	// First use expands; the alias is gone for the second, which goes
	// through as-is.
	if len(sent) != 2 || sent[0] != "cast gate" || sent[1] != "gg" {
		t.Fatalf("expected [cast gate, gg], got %v", sent)
	}
}

func TestRegistryRemovalDuringDispatchIsHonored(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = {}
		rune.hooks.on("output", function()
			fired[#fired + 1] = "first"
			rune.hooks.remove("victim")
		end, { priority = 10 })
		rune.hooks.on("output", function()
			fired[#fired + 1] = "victim"
		end, { name = "victim", priority = 20 })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("anything"))

	// The snapshot keeps iteration stable, but active() must skip an
	// entry removed earlier in the same dispatch.
	assertLua(t, engine, `
		assert(#fired == 1 and fired[1] == "first",
			"removed handler must not fire; fired: " .. table.concat(fired, ","))
	`)
}

func TestRegistryAdditionDuringDispatchWaitsForNext(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = {}
		registered = false
		rune.hooks.on("output", function()
			fired[#fired + 1] = "adder"
			if not registered then
				registered = true
				-- Priority 5 would sort BEFORE this handler; it must
				-- still not run during the current dispatch.
				rune.hooks.on("output", function()
					fired[#fired + 1] = "added"
				end, { priority = 5 })
			end
		end, { priority = 10 })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("first"))
	assertLua(t, engine, `
		assert(table.concat(fired, ",") == "adder",
			"added handler ran in the same dispatch: " .. table.concat(fired, ","))
	`)

	engine.OnOutput(text.NewLine("second"))
	assertLua(t, engine, `
		assert(table.concat(fired, ",") == "adder,added,adder",
			"next dispatch order wrong: " .. table.concat(fired, ","))
	`)
}

// TestRegistryChangesMarkOnlyTheirSnapshotStale verifies on_change routing:
// bind and bar registries mark their own UI snapshot stale on every change to
// their active set, dispatch-only registries mark nothing, and no-op changes
// notify nobody.
func TestRegistryChangesMarkOnlyTheirSnapshotStale(t *testing.T) {
	tests := []struct {
		name                string
		setup, action       string
		binds, layout, bars int
	}{
		{name: "trigger disable", setup: `h = rune.trigger.contains("x", "look")`, action: `h:disable()`},
		{name: "alias disable", setup: `rune.alias.exact("n", "north")`, action: `rune.alias.disable("n")`},
		{name: "bind add", action: `rune.bind("f5", function() end)`, binds: 1},
		{name: "bind disable", setup: `h = rune.bind("f5", function() end)`, action: `h:disable()`, binds: 1},
		{name: "bind disable no-op", setup: `h = rune.bind("f5", function() end); h:disable()`, action: `h:disable()`},
		{name: "unbind", setup: `rune.bind("f5", function() end)`, action: `rune.unbind("f5")`, binds: 1},
		{name: "bar add", action: `rune.ui.bar("x", function() return "x" end)`, bars: 1},
		{name: "bar toggle", setup: `rune.ui.bar("x", function() return "x" end)`, action: `rune.bars.toggle("x")`, bars: 1},
		{name: "refresh bars", action: `rune.ui.refresh_bars()`, bars: 1},
		{name: "group with a bind", setup: `rune.bind("f6", function() end, {group = "g"})`, action: `rune.group.disable("g")`, binds: 1},
		{name: "group with a bar", setup: `rune.ui.bar("x", function() return "x" end, {group = "g"})`, action: `rune.group.disable("g")`, bars: 1},
		{name: "group with only a trigger", setup: `rune.trigger.contains("x", "look", {group = "g"})`, action: `rune.group.disable("g")`},
		{name: "group enable no-op", setup: `rune.bind("f6", function() end, {group = "g"})`, action: `rune.group.enable("g")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString("setup", tt.setup); err != nil {
				t.Fatal(err)
			}
			host.DrainSnapshotChanges()
			if err := engine.DoString("action", tt.action); err != nil {
				t.Fatal(err)
			}
			binds, layout, bars := host.DrainSnapshotChanges()
			if binds != tt.binds || layout != tt.layout || bars != tt.bars {
				t.Fatalf("stale binds/layout/bars = %d/%d/%d, want %d/%d/%d",
					binds, layout, bars, tt.binds, tt.layout, tt.bars)
			}
		})
	}
}

// A trigger added during dispatch at a priority that sorts after the running
// one must not join the current pass: registries replace their item lists on
// change, so the dispatch loop keeps iterating the list it started with.
func TestRegistryAdditionSortedLaterWaitsForNext(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `
		fired = {}
		registered = false
		rune.trigger.contains("line", function()
			fired[#fired + 1] = "adder"
			if not registered then
				registered = true
				rune.trigger.contains("line", function()
					fired[#fired + 1] = "added"
				end, { priority = 90 })
			end
		end, { priority = 10 })
	`); err != nil {
		t.Fatal(err)
	}

	engine.OnOutput(text.NewLine("line one"))
	assertLua(t, engine, `
		assert(table.concat(fired, ",") == "adder",
			"added trigger ran in the same dispatch: " .. table.concat(fired, ","))
	`)

	engine.OnOutput(text.NewLine("line two"))
	assertLua(t, engine, `
		assert(table.concat(fired, ",") == "adder,adder,added",
			"next dispatch order wrong: " .. table.concat(fired, ","))
	`)
}

// Regression: an invalid option on re-registration deleted the existing
// entry (the name upsert ran first), then failed with a sort error.
func TestRegistryInvalidOptionsLeaveExistingEntry(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		rune.timer.every(60, function() end, { name = "tick" })
		local ok, err = pcall(rune.timer.every, 30, function() end, { name = "tick", priority = "high" })
		assert(not ok, "invalid priority must raise")
		assert(tostring(err):find("rune.timer.every: opts.priority must be a number", 1, true), tostring(err))
		assert(rune.timer.get("tick"), "existing timer must survive a rejected re-registration")
	`)
}

// Constructors reject arguments that could never work, naming the call.
func TestRegistryConstructorsRejectBadArguments(t *testing.T) {
	cases := []struct{ call, want string }{
		{`rune.alias.exact("kill", nil)`, "rune.alias.exact: action must be a function or string"},
		{`rune.alias.regex("^k$", 42)`, "rune.alias.regex: action must be a function or string"},
		{`rune.trigger.contains("x", 42)`, "rune.trigger.contains: action must be a function, string, or nil"},
		{`rune.timer.after(-1, function() end)`, "rune.timer.after: seconds must be a positive number"},
		{`rune.timer.every("5", function() end)`, "rune.timer.every: seconds must be a positive number"},
		{`rune.timer.after(5, nil)`, "rune.timer.after: action must be a function or string"},
		{`rune.hooks.on(42, function() end)`, "rune.hooks.on: event must be a string"},
		{`rune.hooks.on("output", "look")`, "rune.hooks.on: handler must be a function"},
		{`rune.gmcp.on(nil, function() end)`, "rune.gmcp.on: package must be a string"},
		{`rune.gmcp.on("Char", nil)`, "rune.gmcp.on: handler must be a function"},
		{`rune.trigger.contains("x", "y", { group = 7 })`, "rune.trigger.contains: opts.group must be a string"},
		{`rune.alias.regex("^k$", "kill", { once = "yes" })`, "rune.alias.regex: opts.once must be a boolean"},
	}
	for _, tc := range cases {
		t.Run(tc.call, func(t *testing.T) {
			engine, _, cleanup := setupTest(t)
			defer cleanup()
			err := engine.DoString("user_script", tc.call)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "user_script") {
				t.Fatalf("error %q does not point at the caller", err)
			}
		})
	}
}

// Removing an entry twice is a no-op the second time: no second
// on_remove or on_change for an entry that is already gone.
func TestRegistryRemoveIsIdempotent(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `_G.h = rune.bind("f5", function() end)`)
	host.DrainSnapshotChanges()
	assertLua(t, engine, `h:remove(); h:remove(); h:disable(); h:enable()`)
	if binds, _, _ := host.DrainSnapshotChanges(); binds != 1 {
		t.Fatalf("binds changed %d times, want 1", binds)
	}
}

// Regression: enable() is documented to reset the failure count, but a
// quarantined entry re-enabled was disabled again on its next failure.
func TestRegistryEnableResetsFailures(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		rune.trigger.contains("boom", function() error("broken") end, { name = "bad" })
	`)
	for range 3 {
		engine.OnOutput(text.NewLine("boom"))
	}
	assertLua(t, engine, `
		assert(not rune.trigger.get("bad")._data.enabled, "three failures must quarantine")
		rune.trigger.enable("bad")
	`)
	engine.OnOutput(text.NewLine("boom"))
	host.DrainPrintCalls()
	assertLua(t, engine, `
		assert(rune.trigger.get("bad")._data.enabled, "one failure after enable() must not quarantine again")
	`)
}

// Everything the embedded core registers is a core default; anything
// registered afterwards is not.
func TestRegistryCoreEntriesAreSealedAtBoot(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		local namespaces = { rune.hooks, rune.binds, rune.bars, rune.timer,
			rune.alias, rune.trigger, rune.command, rune.gmcp }
		for _, ns in ipairs(namespaces) do
			for _, row in ipairs(ns.list()) do
				assert(row.core, "boot entry not core: " .. tostring(row.name or row.event or row.key))
			end
		end
		rune.trigger.contains("x", "y", { name = "mine" })
		for _, row in ipairs(rune.trigger.list()) do
			if row.name == "mine" then assert(not row.core, "user entry marked core") end
		end
		assert(rune.timer.count() == 0, "the core registers no timers")
	`)
}

// Regression: hooks.clear() removed the core trigger dispatcher, so every
// trigger silently stopped. clear() now removes only user entries and
// restores Rune's own.
func TestRegistryClearRestoresCoreDefaults(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		rune.trigger.contains("food", "eat food")
		rune.hooks.on("output", function() end, { name = "mine" })
		rune.hooks.clear()
		assert(rune.hooks.get("mine") == nil, "user hook survived clear()")
		assert(rune.hooks.count() > 0, "core hooks must be restored")
	`)
	engine.OnOutput(text.NewLine("some food"))
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "eat food" {
		t.Fatalf("triggers stopped after hooks.clear(): %v", sent)
	}

	// clear(event) touches only that event; a removed or disabled core
	// entry comes back enabled with its failures forgiven.
	assertLua(t, engine, `
		rune.hooks.on("echo", function() end, { name = "echo-mine" })
		rune.hooks.on("output", function() end, { name = "out-mine" })
		local core
		for _, h in ipairs(rune.hooks.list()) do
			if h.event == "output" and h.core then core = h end
		end
		assert(core, "no core output hook")
		rune.hooks.clear("output")
		assert(rune.hooks.get("out-mine") == nil, "clear(event) kept a user hook on that event")
		assert(rune.hooks.get("echo-mine"), "clear(event) removed a hook on another event")
	`)
}

// Every registry namespace has the same management set, and handles and
// groups can toggle.
func TestRegistryUniformManagement(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		local namespaces = {
			hooks = rune.hooks, binds = rune.binds, bars = rune.bars, timer = rune.timer,
			alias = rune.alias, trigger = rune.trigger, command = rune.command, gmcp = rune.gmcp,
		}
		for label, ns in pairs(namespaces) do
			for _, verb in ipairs({ "get", "enable", "disable", "toggle", "remove",
					"list", "count", "clear", "remove_group" }) do
				assert(type(ns[verb]) == "function", label .. "." .. verb .. " missing")
			end
		end

		local h = rune.trigger.contains("x", "y", { name = "t", group = "g" })
		h:toggle()
		assert(not h._data.enabled, "handle toggle did not disable")
		assert(rune.trigger.toggle("t") and h._data.enabled, "namespace toggle did not enable")
		rune.group.toggle("g")
		assert(not rune.group.is_enabled("g"), "group toggle did not disable")
		rune.group.toggle("g")
		assert(rune.group.is_enabled("g"), "group toggle did not enable")
	`)
}

// An option no registry understands is ignored with a notice, which
// catches typos; the registration still succeeds.
func TestRegistryUnknownOptionNotice(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.DrainPrintCalls()
	assertLua(t, engine, `assert(rune.trigger.contains("x", "y", { gagg = true }))`)
	printed := strings.Join(host.DrainPrintCalls(), "\n")
	if !strings.Contains(printed, "unknown option 'gagg'") {
		t.Fatalf("no notice for unknown option: %q", printed)
	}
	host.DrainPrintCalls()
	assertLua(t, engine, `rune.trigger.contains("x", "y", { gag = true, priority = 10, once = true, group = "g" })`)
	if printed := host.DrainPrintCalls(); len(printed) != 0 {
		t.Fatalf("known options produced output: %q", printed)
	}
}
