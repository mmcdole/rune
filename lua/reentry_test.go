package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
)

// TestLoadCanReenterLuaAndReuseOuterExecution verifies the other production
// reentry path: rune.load enters a child chunk, the child calls another native
// function, and the original callback then dispatches the loaded hook before
// returning to its caller.
func TestLoadCanReenterLuaAndReuseOuterExecution(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	dir := t.TempDir()
	child := filepath.Join(dir, "child.lua")
	if err := os.WriteFile(
		child,
		[]byte(`rune.send_raw("child")`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	host.DrainPrintCalls()

	code := fmt.Sprintf(`
		rune.hooks.on("loaded", function()
			rune.send_raw("loaded")
		end)
		local ok, err = rune.load(%q)
		assert(ok, err)
		rune.send_raw("outer")
	`, child)
	if err := engine.DoString("load reentry", code); err != nil {
		t.Fatalf("load through native callback: %v", err)
	}

	want := []string{"child", "loaded", "outer"}
	if got := host.DrainNetworkCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reentrant load sends = %q, want %q", got, want)
	}
	for _, printed := range host.DrainPrintCalls() {
		if strings.Contains(printed, "state is executing") ||
			strings.Contains(printed, "[Error]") {
			t.Fatalf("reentrant load reported error: %q", printed)
		}
	}
}

// configChangeReentryHost mirrors Session.OnConfigChange: a Lua-side
// configuration mutation synchronously asks the same engine for its updated
// binds and bars before returning to the script that made the mutation.
type configChangeReentryHost struct {
	*MockHost
	engine *Engine
	active bool

	boundKeys []string
	bars      map[string]ui.BarContent
}

func (h *configChangeReentryHost) OnConfigChange() {
	if !h.active {
		return
	}
	h.boundKeys = h.engine.GetBoundKeys()
	h.bars = h.engine.RenderBars(80)
}

// TestConfigChangeCanReenterLuaAndResume verifies the real Session call path:
// Lua -> Go OnConfigChange -> Lua bind/bar queries -> return to the outer Lua
// invocation. The nested calls must observe the mutation, and the outer script
// must resume after the host callback returns.
func TestConfigChangeCanReenterLuaAndResume(t *testing.T) {
	engine, host := newConfigChangeReentryEngine(t)

	if err := engine.DoString("reentry setup", `
		rune.ui.bar("reentry", function(width)
			return "width=" .. width
		end)
	`); err != nil {
		t.Fatalf("register bar: %v", err)
	}
	host.DrainPrintCalls()
	host.active = true

	if err := engine.DoString("reentry", `
		rune.bind("ctrl+shift+r", function() end)
		rune.send_raw("outer resumed")
	`); err != nil {
		t.Fatalf("configuration mutation: %v", err)
	}

	if !containsString(host.boundKeys, "ctrl+shift+r") {
		t.Errorf("nested bind query did not observe new binding: %q", host.boundKeys)
	}
	if got := host.bars["reentry"].Left; got != "width=80" {
		t.Errorf("nested bar render = %q, want %q", got, "width=80")
	}
	if got := host.DrainNetworkCalls(); !reflect.DeepEqual(got, []string{"outer resumed"}) {
		t.Errorf("outer script did not resume: sends = %q", got)
	}
	if got := host.DrainPrintCalls(); len(got) != 0 {
		t.Errorf("nested calls reported errors: %q", got)
	}
}

// TestConfigChangeCanReenterFromCoroutine verifies that ordinary Engine calls
// made by the host remain bound to the Lua thread that entered Go rather than
// silently using the VM's main thread.
func TestConfigChangeCanReenterFromCoroutine(t *testing.T) {
	engine, host := newConfigChangeReentryEngine(t)
	if err := engine.DoString("coroutine setup", `
		worker = false
		rune.ui.bar("thread", function()
			if coroutine.running() == worker then
				return "worker"
			end
			return "wrong thread"
		end)
	`); err != nil {
		t.Fatalf("register coroutine-aware bar: %v", err)
	}
	host.active = true

	if err := engine.DoString("coroutine reentry", `
			worker = coroutine.create(function()
				rune.bind("ctrl+shift+c", function() end)
			rune.send_raw("coroutine resumed")
		end)
		local ok, err = coroutine.resume(worker)
		assert(ok, err)
		assert(coroutine.status(worker) == "dead")
	`); err != nil {
		t.Fatalf("configuration mutation from coroutine: %v", err)
	}

	if !containsString(host.boundKeys, "ctrl+shift+c") {
		t.Errorf("nested bind query used the wrong thread: %q", host.boundKeys)
	}
	if got := host.bars["thread"].Left; got != "worker" {
		t.Errorf("nested bar render ran on %q, want worker coroutine", got)
	}
	if got := host.DrainNetworkCalls(); !reflect.DeepEqual(
		got,
		[]string{"coroutine resumed"},
	) {
		t.Errorf("coroutine did not resume: sends = %q", got)
	}
	if got := host.DrainPrintCalls(); len(got) != 0 {
		t.Errorf("coroutine reentry reported errors: %q", got)
	}
}

// TestReentryFailureUsesActiveFrame verifies that an error produced by a
// nested call reaches the error hook through the active callback frame instead
// of trying to start a second outer execution.
func TestReentryFailureUsesActiveFrame(t *testing.T) {
	engine, host := newConfigChangeReentryEngine(t)
	if err := engine.DoString("failure setup", `
		rune.hooks.on("error", function(message)
			rune.send_raw("reported:" .. message)
		end, {priority = 1})
		rune.bars._render_all = function()
			error("nested render failure")
		end
	`); err != nil {
		t.Fatalf("install nested failure: %v", err)
	}
	host.DrainPrintCalls()
	host.active = true

	if err := engine.DoString("failed reentry", `
		rune.bind("ctrl+shift+e", function() end)
		rune.send_raw("outer resumed")
	`); err != nil {
		t.Fatalf("outer execution failed: %v", err)
	}

	sent := host.DrainNetworkCalls()
	if len(sent) != 2 ||
		!strings.Contains(sent[0], "reported:bar render:") ||
		!strings.Contains(sent[0], "nested render failure") ||
		sent[1] != "outer resumed" {
		t.Fatalf("nested failure routing = %q", sent)
	}
	for _, printed := range host.DrainPrintCalls() {
		if strings.Contains(printed, "state is executing") {
			t.Fatalf("error reporting left active callback frame: %q", printed)
		}
	}
	if err := engine.DoString("after failed reentry", `
		rune.send_raw("idle state restored")
	`); err != nil {
		t.Fatalf("VM unusable after outer callback returned: %v", err)
	}
	if got := host.DrainNetworkCalls(); !reflect.DeepEqual(
		got,
		[]string{"idle state restored"},
	) {
		t.Fatalf("post-reentry execution = %q", got)
	}
}

func newConfigChangeReentryEngine(
	t *testing.T,
) (*Engine, *configChangeReentryHost) {
	t.Helper()
	host := &configChangeReentryHost{MockHost: NewMockHost()}
	engine := NewEngine(host)
	host.engine = engine
	t.Cleanup(engine.Close)

	if err := engine.Init(); err != nil {
		t.Fatalf("initialize engine: %v", err)
	}
	loadCoreScriptsForReentryTest(t, engine)
	host.DrainPrintCalls()
	return engine, host
}

func loadCoreScriptsForReentryTest(t *testing.T, engine *Engine) {
	t.Helper()
	entries, err := CoreScripts.ReadDir("core")
	if err != nil {
		t.Fatalf("read core scripts: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	for _, file := range files {
		content, err := CoreScripts.ReadFile("core/" + file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if err := engine.DoString(file, string(content)); err != nil {
			t.Fatalf("execute %s: %v", file, err)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
