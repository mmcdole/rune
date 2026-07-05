package lua

import (
	"sort"
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// loadCoreScripts loads the embedded core into a fresh engine, in
// order — what Session.boot does after /reload.
func loadCoreScripts(engine *Engine) error {
	entries, err := CoreScripts.ReadDir("core")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		content, err := CoreScripts.ReadFile("core/" + f)
		if err != nil {
			return err
		}
		if err := engine.DoString(f, string(content)); err != nil {
			return err
		}
	}
	return nil
}

func logContains(host *MockHost, substr string) bool {
	for _, w := range host.LogWrites {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestLogPlainModeStripsANSI(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("t", `rune.log.start("test.log")`); err != nil {
		t.Fatal(err)
	}
	engine.OnOutput(text.NewLine("\x1b[1;32mgreen hello\x1b[m"))

	if !logContains(host, "green hello") {
		t.Fatalf("line not logged: %v", host.LogWrites)
	}
	if logContains(host, "\x1b[1;32m") {
		t.Error("plain mode must strip ANSI codes")
	}
}

func TestLogRawModeKeepsANSI(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("t", `rune.log.start("test.log", { raw = true })`); err != nil {
		t.Fatal(err)
	}
	engine.OnOutput(text.NewLine("\x1b[1;32mgreen hello\x1b[m"))

	if !logContains(host, "\x1b[1;32mgreen hello\x1b[m") {
		t.Errorf("raw mode must keep ANSI codes: %v", host.LogWrites)
	}
}

func TestLogCommandStartRaw(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnInput("/log start raw quest.log")
	if path, active := host.LogStatus(); !active || path != "quest.log" {
		t.Fatalf("expected active log at quest.log, got %q active=%v", path, active)
	}
	engine.OnOutput(text.NewLine("\x1b[35mmagenta\x1b[m"))
	if !logContains(host, "\x1b[35m") {
		t.Error("/log start raw should keep ANSI codes")
	}

	engine.OnInput("/log stop")
	engine.OnInput("/log start plain.log")
	host.LogWrites = nil
	engine.OnOutput(text.NewLine("\x1b[35mmagenta\x1b[m"))
	if logContains(host, "\x1b[35m") {
		t.Error("a later plain log must strip again")
	}
}

func TestLogRawModeSurvivesReload(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("t", `rune.log.start("test.log", { raw = true })`); err != nil {
		t.Fatal(err)
	}

	// /reload tears down the VM; the Go file handle and the session
	// store survive. Rebuild the engine over the same host, as
	// Session.boot does, and check the mode was restored.
	engine.Close()
	engine2 := NewEngine(host)
	if err := engine2.Init(); err != nil {
		t.Fatal(err)
	}
	if err := loadCoreScripts(engine2); err != nil {
		t.Fatal(err)
	}
	defer engine2.Close()

	host.LogWrites = nil
	engine2.OnOutput(text.NewLine("\x1b[1;34mblue after reload\x1b[m"))
	if !logContains(host, "\x1b[1;34m") {
		t.Errorf("raw mode must survive reload: %v", host.LogWrites)
	}
}

func TestLogStartBadOptsRaises(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("t", `rune.log.start("x.log", "raw")`); err == nil {
		t.Error("non-table opts should raise")
	}
}
