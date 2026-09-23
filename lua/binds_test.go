package lua

import "testing"

// TestKeyBindRoundTrip verifies the bind path: Lua registers through
// rune.bind, Go sees the key via GetBindings, and HandleKeyBind
// dispatches back into the Lua callback. Unbinding removes the key.
func TestKeyBindRoundTrip(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `rune.bind("ctrl+g", function() rune.send_raw("bound") end)`); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	found := false
	for k := range engine.GetBindings() {
		if k == "ctrl+g" {
			found = true
		}
	}
	if !found {
		t.Fatal("ctrl+g missing from GetBindings")
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
	for k := range engine.GetBindings() {
		if k == "ctrl+g" {
			t.Error("ctrl+g still bound after unbind")
		}
	}
}

func TestEditorBindingsShareRegistry(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
        assert(rune.input.keys == nil)
        assert(rune.binds.get("enter"):action() == "input.submit")
        local handles = rune.bind({"f1", "f2"}, "input.newline", {group="editing"})
        assert(#handles == 2)
        handles[1]:remove()
        assert(rune.binds.get("f1") == nil)
        assert(rune.binds.get("f2"):action() == "input.newline")
        rune.bind("enter", function() rune.send_raw("callback") end)
    `)
	if b := engine.GetBindings()["enter"]; b.Action != "" || !b.Enabled {
		t.Fatalf("replacement = %+v", b)
	}
	engine.HandleKeyBind("enter")
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "callback" {
		t.Fatalf("replacement callback: %v", sent)
	}
	assertLua(t, engine, `rune.group.disable("editing")`)
	if engine.GetBindings()["f2"].Enabled {
		t.Fatal("disabled group remained active")
	}
	assertLua(t, engine, `rune.group.enable("editing"); rune.binds.get("f2"):disable()`)
	if engine.GetBindings()["f2"].Enabled {
		t.Fatal("disabled handle remained active")
	}
	assertLua(t, engine, `rune.binds.clear()`)
	if bindings := engine.GetBindings(); bindings == nil || len(bindings) != 0 {
		t.Fatalf("clear must publish an empty snapshot: %+v", bindings)
	}
	if err := engine.Init(); err != nil {
		t.Fatal(err)
	}
	loadTestCoreScripts(t, engine)
	if !engine.GetBindings().Matches("submit", "enter") {
		t.Fatal("reload did not restore defaults")
	}
}

func TestBindingArraysValidateBeforeMutation(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	for _, code := range []string{
		`rune.bind({"enter", false}, "input.newline")`,
		`rune.bind({[1]="enter", [3]="f1"}, "input.newline")`,
		`rune.bind({"enter", label="bad"}, "input.newline")`,
		`rune.bind({}, "input.newline")`,
		`rune.bind("enter", "input.unknown")`,
		`rune.bind({"enter", "f1"}, "input.newline", false)`,
		`rune.bind({"enter", "f1"}, "input.newline", {name = "shared"})`,
	} {
		if err := engine.DoString("invalid bind", code); err == nil {
			t.Fatalf("accepted %s", code)
		}
		if !engine.GetBindings().Matches("submit", "enter") {
			t.Fatalf("partial mutation: %s", code)
		}
	}
}
