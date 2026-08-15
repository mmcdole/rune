package lua

import (
	"reflect"
	"testing"
)

func TestConfigGetReturnsGoDefaults(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("config defaults", `
		assert(rune.config.get("command_separator") == ";")
		assert(rune.config.get("history_character") == "!")
		assert(rune.config.get("keep_input") == false)
		assert(rune.config.get("numpad") == false)
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
		rune.config.set("numpad", true)
		assert(rune.config.get("numpad") == true)
	`); err != nil {
		t.Fatalf("set recognized config value: %v", err)
	}
}

func TestConfigCommandSeparatorDrivesCommandSplitting(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("set command separator", `
		rune.config.set("command_separator", "|")
		rune.send("north|east")
	`); err != nil {
		t.Fatalf("use configured command separator: %v", err)
	}
	if got, want := host.DrainNetworkCalls(), []string{"north", "east"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands sent with configured command separator = %q, want %q", got, want)
	}
}

func TestConfigHistoryCharacterAcceptsCharacterOrEmpty(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("set history character", `
		rune.config.set("history_character", "?")
		assert(rune.config.get("history_character") == "?")
		rune.config.set("history_character", "¿")
		assert(rune.config.get("history_character") == "¿")
		rune.config.set("history_character", "")
		assert(rune.config.get("history_character") == "")
	`); err != nil {
		t.Fatalf("set history character: %v", err)
	}
}

func TestConfigRejectsInvalidHistoryCharacterWithoutMutation(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CommitConfig()
	host.DrainConfigChanges()

	if err := engine.DoString("reject invalid history character", `
		rune.config.set("history_character", "?")
		local invalid = {
			"two",
			" ",
			"\n",
			"/",
			string.char(0),
			string.char(255),
			string.char(0xCC, 0x81),       -- combining acute accent
			string.char(0xEF, 0xB8, 0x8F), -- variation selector-16
		}
		for _, value in ipairs(invalid) do
			assert(not pcall(rune.config.set, "history_character", value))
			assert(rune.config.get("history_character") == "?")
		end
		assert(not pcall(rune.config.set, "history_character", false))
		assert(rune.config.get("history_character") == "?")
	`); err != nil {
		t.Fatalf("validate history character before mutation: %v", err)
	}

	want := []Config{{CommandSeparator: ";", HistoryCharacter: "?", KeepInput: false}}
	if got := host.DrainConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("history character publications = %+v, want %+v", got, want)
	}
}

func TestConfigUnchangedHistoryCharacterDoesNotPublish(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CommitConfig()
	host.DrainConfigChanges()

	if err := engine.DoString("keep history character", `
		rune.config.set("history_character", "!")
	`); err != nil {
		t.Fatalf("keep history character: %v", err)
	}
	if got := host.DrainConfigChanges(); len(got) != 0 {
		t.Fatalf("unchanged history character published config: %+v", got)
	}
}

func TestConfigRejectsEmptyCommandSeparatorWithoutMutation(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("reject empty command separator", `
		rune.config.set("command_separator", "|")
		local ok = pcall(rune.config.set, "command_separator", "")
		assert(not ok)
		assert(rune.config.get("command_separator") == "|")
	`); err != nil {
		t.Fatalf("validate command separator before mutation: %v", err)
	}
}

func TestConfigRejectsNonStringCommandSeparatorWithoutCoercion(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("reject numeric command separator", `
		local ok = pcall(rune.config.set, "command_separator", 42)
		assert(not ok)
		assert(rune.config.get("command_separator") == ";")
	`); err != nil {
		t.Fatalf("validate command separator type before mutation: %v", err)
	}
}

func TestConfigBootPublishesOneFinalTypedConfig(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("staged config", `
		rune.config.set("command_separator", "|")
		rune.config.set("history_character", "^")
		rune.config.set("keep_input", true)
		rune.config.set("numpad", true)
	`); err != nil {
		t.Fatalf("stage config during boot: %v", err)
	}
	if got := host.DrainConfigChanges(); len(got) != 0 {
		t.Fatalf("config published before boot commit: %+v", got)
	}

	engine.CommitConfig()
	engine.CommitConfig()

	want := []Config{{CommandSeparator: "|", HistoryCharacter: "^", KeepInput: true, Numpad: true}}
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
	want := []Config{{CommandSeparator: ";", HistoryCharacter: "!", KeepInput: true}}
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

	want := []Config{{CommandSeparator: ";", HistoryCharacter: "!", KeepInput: true}}
	if got := host.DrainConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime config publications = %+v, want %+v", got, want)
	}
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("config update invalidated presentation %d times", got)
	}
}

func TestConfigNumpadRuntimeChangePublishesImmediately(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	engine.CommitConfig()
	host.DrainConfigChanges()

	if err := engine.DoString("enable numpad", `
		rune.config.set("numpad", true)
	`); err != nil {
		t.Fatalf("change numpad config: %v", err)
	}

	want := []Config{{CommandSeparator: ";", HistoryCharacter: "!", Numpad: true}}
	if got := host.DrainConfigChanges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("numpad config publications = %+v, want %+v", got, want)
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
		ok = pcall(rune.config.set, "numpad", "yes")
		assert(not ok)
		assert(rune.config.get("numpad") == false)
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
		assert(rune.config.get("command_separator") == ";")
		assert(rune.config.get("history_character") == "!")
		assert(rune.config.get("keep_input") == false)
		assert(rune.config.get("numpad") == false)
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
