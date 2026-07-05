package lua

// Tests for the shared registry factory (15_registry.lua), exercised
// through the public trigger/alias/hook APIs the way user scripts use
// it: upsert-by-name, priority ordering, once, and mutation during
// dispatch (the snapshot guarantee).

import (
	"testing"

	"github.com/mmcdole/rune/text"
)

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

	engine.OnInput("gg")
	engine.OnInput("gg")

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
