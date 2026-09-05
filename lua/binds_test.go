package lua

import "testing"

// TestKeyBindRoundTrip verifies the bind path: Lua registers through
// rune.bind, Go sees the key via GetBoundKeys, and HandleKeyBind
// dispatches back into the Lua callback. Unbinding removes the key.
func TestKeyBindRoundTrip(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `rune.bind("ctrl+g", function() rune.send_raw("bound") end)`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	found := false
	for _, k := range engine.GetBoundKeys() {
		if k == "ctrl+g" {
			found = true
		}
	}
	if !found {
		t.Fatal("ctrl+g missing from GetBoundKeys")
	}

	engine.HandleKeyBind("ctrl+g")
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "bound" {
		t.Errorf("expected bind callback to fire, got %v", sent)
	}

	// An unbound key is a no-op, not an error
	engine.HandleKeyBind("ctrl+q")
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("unbound key fired something: %v", sent)
	}

	if err := engine.DoString("unbind", `assert(rune.unbind("ctrl+g"))`); err != nil {
		t.Fatalf("unbind failed: %v", err)
	}
	for _, k := range engine.GetBoundKeys() {
		if k == "ctrl+g" {
			t.Error("ctrl+g still bound after unbind")
		}
	}
}
