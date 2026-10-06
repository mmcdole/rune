package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

// connectReentryHost mirrors Session.Connect: rune.connect() synchronously
// runs the connecting hook through the same engine before returning to the
// script that requested the connection.
type connectReentryHost struct {
	*MockHost
	engine *Engine
}

func (h *connectReentryHost) Connect(address string) {
	h.engine.NotifyConnecting(address)
}

// TestConnectCanReenterLuaAndResume verifies the real Session call path:
// Lua -> Go Connect -> Lua connecting hook -> outer Lua invocation. The nested
// hook must run, and the outer script must resume after the host returns.
func TestConnectCanReenterLuaAndResume(t *testing.T) {
	engine, host := newConnectReentryEngine(t)

	if err := engine.DoString("reentry", `
		rune.hooks.on("connecting", function(address)
			rune.send_raw("connecting " .. address)
		end)
		rune.connect("example.org:23")
		rune.send_raw("outer resumed")
	`); err != nil {
		t.Fatalf("connect: %v", err)
	}

	want := []string{"connecting example.org:23", "outer resumed"}
	if got := host.DrainNetworkCalls(); !reflect.DeepEqual(got, want) {
		t.Errorf("reentrant connect sends = %q, want %q", got, want)
	}
	for _, printed := range host.DrainPrintCalls() {
		if strings.Contains(printed, "[Error]") {
			t.Errorf("nested calls reported errors: %q", printed)
		}
	}
}

// TestConnectCanReenterFromCoroutine verifies that ordinary Engine calls
// made by the host remain bound to the Lua thread that entered Go rather than
// silently using the VM's main thread.
func TestConnectCanReenterFromCoroutine(t *testing.T) {
	engine, host := newConnectReentryEngine(t)

	if err := engine.DoString("coroutine reentry", `
		worker = false
		rune.hooks.on("connecting", function()
			if coroutine.running() == worker then
				rune.send_raw("worker")
			else
				rune.send_raw("wrong thread")
			end
		end)
		worker = coroutine.create(function()
			rune.connect("example.org:23")
			rune.send_raw("coroutine resumed")
		end)
		local ok, err = coroutine.resume(worker)
		assert(ok, err)
		assert(coroutine.status(worker) == "dead")
	`); err != nil {
		t.Fatalf("connect from coroutine: %v", err)
	}

	want := []string{"worker", "coroutine resumed"}
	if got := host.DrainNetworkCalls(); !reflect.DeepEqual(got, want) {
		t.Errorf("coroutine reentry sends = %q, want %q", got, want)
	}
	for _, printed := range host.DrainPrintCalls() {
		if strings.Contains(printed, "[Error]") {
			t.Errorf("coroutine reentry reported errors: %q", printed)
		}
	}
}

// TestReentryFailureUsesActiveFrame verifies that an error produced by a
// nested call reaches the error hook through the active callback frame instead
// of trying to start a second outer execution.
func TestReentryFailureUsesActiveFrame(t *testing.T) {
	engine, host := newConnectReentryEngine(t)
	if err := engine.DoString("failure setup", `
		rune.hooks.on("error", function(message)
			rune.send_raw("reported:" .. message)
		end, {priority = 1})
		local call = rune.hooks.call
		rune.hooks.call = function(event, ...)
			if event == "connecting" then
				error("nested hook failure")
			end
			return call(event, ...)
		end
	`); err != nil {
		t.Fatalf("install nested failure: %v", err)
	}
	host.DrainPrintCalls()

	if err := engine.DoString("failed reentry", `
		rune.connect("example.org:23")
		rune.send_raw("outer resumed")
	`); err != nil {
		t.Fatalf("outer execution failed: %v", err)
	}

	sent := host.DrainNetworkCalls()
	if len(sent) < 2 ||
		!strings.HasPrefix(sent[0], "reported:'connecting' hook:") ||
		!strings.Contains(strings.Join(sent[:len(sent)-1], "\n"), "nested hook failure") ||
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

func newConnectReentryEngine(
	t *testing.T,
) (*Engine, *connectReentryHost) {
	t.Helper()
	host := &connectReentryHost{MockHost: NewMockHost()}
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
