package lua

import "testing"

// TestSessionStoreSurvivesReload verifies rune.session round-trips
// through the host and survives a VM teardown (the whole point).
func TestSessionStoreSurvivesReload(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	script := `
		old_vm_marker = true
		rune.session.set("last_address", "mud.example.com:4000")
		assert(rune.session.get("last_address") == "mud.example.com:4000")
		assert(rune.session.get("missing") == nil)
		rune.session.set("gone", "x")
		rune.session.delete("gone")
		assert(rune.session.get("gone") == nil)
	`
	if err := engine.DoString("session_test", script); err != nil {
		t.Fatalf("session store round-trip failed: %v", err)
	}

	// Tear down and rebuild the VM, as /reload does
	if err := engine.Init(); err != nil {
		t.Fatalf("re-init failed: %v", err)
	}
	loadTestCoreScripts(t, engine)
	if err := engine.DoString("after reload", `
		assert(old_vm_marker == nil, "reload reused the old Lua globals")
		assert(rune.session.get("last_address") == "mud.example.com:4000", "session value lost across reload")
		assert(rune.session.get("gone") == nil, "deleted value restored by reload")
		rune.session.set("after_reload", "new value")
		assert(rune.session.get("after_reload") == "new value", "session store unusable after reload")
	`); err != nil {
		t.Fatalf("session store access after reload: %v", err)
	}
}

// TestStoreRoundTripsStructuredValues verifies rune.store converts
// Lua values to JSON and back: nested tables, arrays, scalars; and
// that unstorable values are rejected with nil, err instead of raised.
func TestStoreRoundTripsStructuredValues(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	script := `
		assert(rune.store.set("cfg", {
			name = "arctic",
			hp_warn = 0.25,
			auto = true,
			route = {"n", "e", "e"},
			nested = { deep = { value = 42 } },
		}))

		local cfg = rune.store.get("cfg")
		assert(cfg.name == "arctic")
		assert(cfg.hp_warn == 0.25)
		assert(cfg.auto == true)
		assert(#cfg.route == 3 and cfg.route[2] == "e")
		assert(cfg.nested.deep.value == 42)

		assert(rune.store.get("missing") == nil)

		-- Unstorable: functions
		local ok, err = rune.store.set("bad", { fn = function() end })
		assert(ok == nil and err ~= nil, "function value should be rejected")

		-- Unstorable: cycles
		local cyc = {}
		cyc.self = cyc
		local ok2, err2 = rune.store.set("bad", cyc)
		assert(ok2 == nil and err2 ~= nil, "cycle should be rejected")

		-- set(key, nil) deletes
		assert(rune.store.set("cfg", nil))
		assert(rune.store.get("cfg") == nil)
	`
	if err := engine.DoString("store_test", script); err != nil {
		t.Fatalf("store round-trip failed: %v", err)
	}
}
