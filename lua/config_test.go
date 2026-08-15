package lua

import (
	"reflect"
	"testing"
)

func TestConfigGetReturnsGoDefaults(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("config defaults", `
		assert(rune.config.get("delimiter") == ";")
		assert(rune.config.get("keep_input") == false)
	`); err != nil {
		t.Fatalf("read config defaults: %v", err)
	}
}

func TestConfigSetUpdatesRecognizedValue(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("set config", `
		rune.config.set("keep_input", true)
		assert(rune.config.get("keep_input") == true)
	`); err != nil {
		t.Fatalf("set recognized config value: %v", err)
	}
}

func TestConfigDelimiterDrivesCommandSplitting(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("set delimiter", `
		rune.config.set("delimiter", "|")
		rune.send("north|east")
	`); err != nil {
		t.Fatalf("use configured delimiter: %v", err)
	}
	if got, want := host.DrainNetworkCalls(), []string{"north", "east"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands sent with configured delimiter = %q, want %q", got, want)
	}
}

func TestConfigRejectsEmptyDelimiterWithoutMutation(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("reject empty delimiter", `
		rune.config.set("delimiter", "|")
		local ok = pcall(rune.config.set, "delimiter", "")
		assert(not ok)
		assert(rune.config.get("delimiter") == "|")
	`); err != nil {
		t.Fatalf("validate delimiter before mutation: %v", err)
	}
}

func TestConfigRejectsNonStringDelimiterWithoutCoercion(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("reject numeric delimiter", `
		local ok = pcall(rune.config.set, "delimiter", 42)
		assert(not ok)
		assert(rune.config.get("delimiter") == ";")
	`); err != nil {
		t.Fatalf("validate delimiter type before mutation: %v", err)
	}
}

func TestConfigBootPublishesOneFinalTypedConfig(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("staged config", `
		rune.config.set("delimiter", "|")
		rune.config.set("keep_input", true)
	`); err != nil {
		t.Fatalf("stage config during boot: %v", err)
	}
	if got := host.DrainConfigChanges(); len(got) != 0 {
		t.Fatalf("config published before boot commit: %+v", got)
	}

	engine.CommitConfig()
	engine.CommitConfig()

	want := []Config{{Delimiter: "|", KeepInput: true}}
	if got := host.DrainConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("committed config publications = %+v, want %+v", got, want)
	}
}

func TestReadyHookParticipatesInStagedConfig(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("ready config", `
		rune.hooks.on("ready", function()
			rune.config.set("keep_input", true)
		end)
	`); err != nil {
		t.Fatal(err)
	}
	engine.NotifyReady()
	if got := host.DrainConfigChanges(); len(got) != 0 {
		t.Fatalf("ready hook published staged config early: %+v", got)
	}

	engine.CommitConfig()
	want := []Config{{Delimiter: ";", KeepInput: true}}
	if got := host.DrainConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ready config publication = %+v, want %+v", got, want)
	}
}

func TestConfigRuntimeChangePublishesTypedConfigImmediately(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CommitConfig()
	host.DrainConfigChanges()
	host.DrainPresentationChanges()

	if err := engine.DoString("runtime config", `
		rune.config.set("keep_input", true)
		rune.config.set("keep_input", true)
	`); err != nil {
		t.Fatalf("change runtime config: %v", err)
	}

	want := []Config{{Delimiter: ";", KeepInput: true}}
	if got := host.DrainConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime config publications = %+v, want %+v", got, want)
	}
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("config update invalidated presentation %d times", got)
	}
}

func TestPresentationChangesDoNotRepublishConfig(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CommitConfig()
	host.DrainConfigChanges()

	if err := engine.DoString("presentation changes", `
		rune.bind("f2", function() end)
		rune.ui.layout({ top = {}, bottom = {} })
	`); err != nil {
		t.Fatal(err)
	}
	if got := host.DrainConfigChanges(); len(got) != 0 {
		t.Fatalf("presentation change republished config: %+v", got)
	}
}

func TestConfigRejectsWrongTypeWithoutMutation(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CommitConfig()
	host.DrainConfigChanges()

	if err := engine.DoString("reject wrong config type", `
		rune.config.set("keep_input", true)
	`); err != nil {
		t.Fatal(err)
	}
	host.DrainConfigChanges()

	if err := engine.DoString("reject wrong config type", `
		local ok = pcall(rune.config.set, "keep_input", "yes")
		assert(not ok)
		assert(rune.config.get("keep_input") == true)
	`); err != nil {
		t.Fatalf("validate config type before mutation: %v", err)
	}
	if got := host.DrainConfigChanges(); len(got) != 0 {
		t.Fatalf("invalid config value published a change: %+v", got)
	}
}

func TestConfigRejectsUnknownKeys(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("reject unknown config", `
		assert(not pcall(rune.config.get, "missing"))
		assert(not pcall(rune.config.set, "missing", true))
		assert(rune.config.get("delimiter") == ";")
		assert(rune.config.get("keep_input") == false)
	`); err != nil {
		t.Fatalf("reject unknown config keys: %v", err)
	}
}

func TestConfigRejectsPropertyAssignment(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("reject config property assignment", `
		local ok, message = pcall(function()
			rune.config.keep_input = true
		end)
		assert(not ok)
		assert(message:find("rune.config.set", 1, true))
		assert(rune.config.get("keep_input") == false)
	`); err != nil {
		t.Fatalf("reject legacy config property assignment: %v", err)
	}
}
