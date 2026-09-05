package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// refreshBarsReentryHost mirrors Session.RefreshBars: rune.ui.refresh_bars()
// synchronously renders bars through the same engine before returning to the
// script that requested the refresh.
type refreshBarsReentryHost struct {
	*MockHost
	engine *Engine
	active bool

	bars map[string]ui.BarContent
}

func (h *refreshBarsReentryHost) RefreshBars() {
	if !h.active {
		return
	}
	h.bars = h.engine.RenderBars(80)
}

// TestRefreshBarsCanReenterLuaAndResume verifies the real Session call path:
// Lua -> Go RefreshBars -> Lua bar render -> outer Lua invocation. The nested
// render must run, and the outer script must resume after the host callback
// returns.
func TestRefreshBarsCanReenterLuaAndResume(t *testing.T) {
	engine, host := newRefreshBarsReentryEngine(t)

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
		rune.ui.refresh_bars()
		rune.send_raw("outer resumed")
	`); err != nil {
		t.Fatalf("bar refresh: %v", err)
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

// TestRefreshBarsCanReenterFromCoroutine verifies that ordinary Engine calls
// made by the host remain bound to the Lua thread that entered Go rather than
// silently using the VM's main thread.
func TestRefreshBarsCanReenterFromCoroutine(t *testing.T) {
	engine, host := newRefreshBarsReentryEngine(t)
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
			rune.ui.refresh_bars()
			rune.send_raw("coroutine resumed")
		end)
		local ok, err = coroutine.resume(worker)
		assert(ok, err)
		assert(coroutine.status(worker) == "dead")
	`); err != nil {
		t.Fatalf("bar refresh from coroutine: %v", err)
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
	engine, host := newRefreshBarsReentryEngine(t)
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
		rune.ui.refresh_bars()
		rune.send_raw("outer resumed")
	`); err != nil {
		t.Fatalf("outer execution failed: %v", err)
	}

	sent := host.DrainNetworkCalls()
	if len(sent) < 2 ||
		!strings.HasPrefix(sent[0], "reported:bar render:") ||
		!strings.Contains(strings.Join(sent[:len(sent)-1], "\n"), "nested render failure") ||
		sent[len(sent)-1] != "outer resumed" {
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

func newRefreshBarsReentryEngine(
	t *testing.T,
) (*Engine, *refreshBarsReentryHost) {
	t.Helper()
	host := &refreshBarsReentryHost{MockHost: NewMockHost()}
	engine := NewEngine(host)
	host.engine = engine
	t.Cleanup(engine.Close)

	if err := engine.Init(); err != nil {
		t.Fatalf("initialize engine: %v", err)
	}
	loadTestCoreScripts(t, engine)
	host.DrainPrintCalls()
	return engine, host
}
