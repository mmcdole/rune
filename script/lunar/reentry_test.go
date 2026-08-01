package lunar

import (
	"testing"

	"github.com/mmcdole/rune/script"
)

// A host callback may synchronously call back into Lua through the ordinary
// Engine surface. Lunar must continue the execution on the callback's Frame;
// attempting to start another outer execution through State fails with
// "lua: state is executing".
func TestEngineCallModuleUsesActiveFrame(t *testing.T) {
	e := New()
	e.RegisterModule("test", map[string]script.GoFunc{
		"invoke": func(c *script.Call) error {
			results, found, err := e.CallModule("test", c.Str(1), 1)
			if err != nil {
				return err
			}
			if !found {
				return c.Errorf("nested function not found")
			}
			c.Return(results[0].String())
			return nil
		},
		"leaf": func(c *script.Call) error {
			c.Return("leaf")
			return nil
		},
	}, nil)
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)

	err := e.DoString("active frame", `
		function test.nested()
			return test.leaf() .. ":nested"
		end
		assert(test.invoke("nested") == "leaf:nested")
	`)
	if err != nil {
		t.Fatalf("ordinary Engine call did not reuse active frame: %v", err)
	}
}

// Reentry must stay on the coroutine that entered the host callback. Shared
// globals alone cannot prove thread affinity, so the nested Lua function
// compares coroutine.running() with the worker itself.
func TestEngineCallModuleKeepsCoroutineAffinity(t *testing.T) {
	e := New()
	e.RegisterModule("test", map[string]script.GoFunc{
		"invoke_bool": func(c *script.Call) error {
			results, found, err := e.CallModule("test", c.Str(1), 1)
			if err != nil {
				return err
			}
			if !found {
				return c.Errorf("nested function not found")
			}
			c.Return(results[0].Bool)
			return nil
		},
	}, nil)
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)

	err := e.DoString("coroutine affinity", `
		local worker
		function test.on_worker()
			return coroutine.running() == worker
		end
		worker = coroutine.create(function()
			assert(test.invoke_bool("on_worker"), "nested call left worker")
		end)
		local ok, message = coroutine.resume(worker)
		assert(ok, message)
		assert(coroutine.status(worker) == "dead")
	`)
	if err != nil {
		t.Fatalf("ordinary Engine call lost coroutine affinity: %v", err)
	}
}

func TestActiveFrameRestoredAfterGoPanic(t *testing.T) {
	e := New()
	e.RegisterModule("test", map[string]script.GoFunc{
		"explode": func(*script.Call) error { panic("nested Go panic") },
	}, nil)
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)

	var callErr error
	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		callErr = e.DoString("Go panic", `test.explode()`)
	}()
	if callErr == nil && panicValue == nil {
		t.Fatal("Go panic disappeared")
	}
	if err := e.DoString("after Go panic", `panic_recovered = true`); err != nil {
		t.Fatalf("State unusable after Go panic: %v", err)
	}
}
