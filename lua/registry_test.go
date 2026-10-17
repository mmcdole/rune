package lua

// Tests for the shared registry factory (20_registry.lua), exercised
// through the public trigger/alias/hook APIs the way user scripts use
// it: upsert-by-name, priority ordering, once, and mutation during
// dispatch (the snapshot guarantee).

import (
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
