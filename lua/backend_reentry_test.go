package lua

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/rune/script"
)

// These tests drive the build-selected script.Engine directly. Reentry is an
// engine contract: ordinary execution methods continue the innermost native
// callback, while calls made after that callback returns start on the idle VM.

func TestScriptEngineReentryUsesActiveCoroutine(t *testing.T) {
	vm := newScriptEngine()
	var sawWorker bool
	vm.RegisterModule("probe", map[string]script.GoFunc{
		"invoke": func(c *script.Call) error {
			results, found, err := vm.CallModule("probe", "is_worker", 1)
			if err != nil {
				return err
			}
			if !found || len(results) != 1 || results[0].Kind != script.KindBool {
				return c.Errorf("nested coroutine probe returned no boolean")
			}
			sawWorker = results[0].Bool
			c.Return(sawWorker)
			return nil
		},
		"leaf": func(c *script.Call) error {
			c.Return("leaf")
			return nil
		},
	}, nil)
	initializeScriptEngine(t, vm)

	err := vm.DoString("coroutine reentry", `
		local worker
		function probe.is_worker()
			assert(probe.leaf() == "leaf")
			return coroutine.running() == worker
		end
		worker = coroutine.create(function()
			assert(probe.invoke(), "nested call left worker coroutine")
			outer_resumed = true
		end)
		local ok, message = coroutine.resume(worker)
		assert(ok, message)
		assert(coroutine.status(worker) == "dead")
		assert(outer_resumed)
	`)
	if err != nil {
		t.Fatalf("coroutine reentry: %v", err)
	}
	if !sawWorker {
		t.Fatal("nested Engine call did not execute on worker coroutine")
	}
	if err := vm.DoString("idle after coroutine", `idle_after_coroutine = true`); err != nil {
		t.Fatalf("idle Engine call after coroutine: %v", err)
	}
}

func TestScriptEngineReentryRecoversFromNestedFailure(t *testing.T) {
	vm := newScriptEngine()
	vm.RegisterModule("probe", map[string]script.GoFunc{
		"fail_go": func(c *script.Call) error {
			return c.Errorf("nested boom")
		},
		"recover": func(c *script.Call) error {
			_, found, nestedErr := vm.CallModule("probe", "fail", 0)
			if !found {
				return c.Errorf("nested failure function not found")
			}
			if nestedErr == nil || !strings.Contains(nestedErr.Error(), "nested boom") {
				return c.Errorf("nested failure = %v", nestedErr)
			}
			if strings.Contains(nestedErr.Error(), "state is executing") {
				return c.Errorf("nested failure left active callback: %v", nestedErr)
			}

			results, found, err := vm.CallModule("probe", "after_failure", 1)
			if err != nil {
				return err
			}
			if !found || len(results) != 1 {
				return c.Errorf("post-failure function unavailable")
			}
			c.Return(results[0].String())
			return nil
		},
	}, nil)
	initializeScriptEngine(t, vm)

	err := vm.DoString("nested failure", `
		function probe.fail()
			probe.fail_go()
		end
		function probe.after_failure()
			return "recovered"
		end
		assert(probe.recover() == "recovered")
		outer_resumed = true
	`)
	if err != nil {
		t.Fatalf("nested failure recovery: %v", err)
	}
	if err := vm.DoString("idle after failure", `assert(outer_resumed)`); err != nil {
		t.Fatalf("idle Engine call after nested failure: %v", err)
	}
}

func TestScriptEngineRejectsLifecycleDuringCallback(t *testing.T) {
	vm := newScriptEngine()
	vm.RegisterModule("probe", map[string]script.GoFunc{
		"lifecycle": func(c *script.Call) error {
			if err := vm.Init(); err == nil || !strings.Contains(err.Error(), "active script call") {
				return c.Errorf("Init during callback = %v", err)
			}

			var closePanic any
			func() {
				defer func() { closePanic = recover() }()
				vm.Close()
			}()
			if closePanic == nil {
				return c.Errorf("Close during callback was not rejected")
			}

			results, found, err := vm.CallModule("probe", "still_alive", 1)
			if err != nil {
				return err
			}
			if !found || len(results) != 1 {
				return c.Errorf("VM detached after lifecycle rejection")
			}
			c.Return(results[0].String())
			return nil
		},
	}, nil)
	initializeScriptEngine(t, vm)

	err := vm.DoString("active lifecycle", `
		function probe.still_alive()
			return "alive"
		end
		assert(probe.lifecycle() == "alive")
		outer_resumed = true
	`)
	if err != nil {
		t.Fatalf("active lifecycle rejection: %v", err)
	}
	if err := vm.DoString("after lifecycle", `assert(outer_resumed)`); err != nil {
		t.Fatalf("VM unusable after lifecycle rejection: %v", err)
	}
}

func TestScriptEngineRejectsLifecycleDuringScopedConsumer(t *testing.T) {
	vm := newScriptEngine()
	initializeScriptEngine(t, vm)
	if err := vm.DoString("scoped lifecycle setup", `
		probe = {}
		function probe.result()
			return {status = "alive"}
		end
	`); err != nil {
		t.Fatalf("scoped lifecycle setup: %v", err)
	}

	found, err := vm.CallModuleScoped(
		"probe",
		"result",
		1,
		nil,
		func(values []script.Value) error {
			if initErr := vm.Init(); initErr == nil ||
				!strings.Contains(initErr.Error(), "active script call") {
				t.Fatalf("Init during scoped consumer = %v", initErr)
			}

			var closePanic any
			func() {
				defer func() { closePanic = recover() }()
				vm.Close()
			}()
			if closePanic == nil {
				t.Fatal("Close during scoped consumer was not rejected")
			}

			if len(values) != 1 || values[0].Table() == nil ||
				values[0].Table().Field("status").Str() != "alive" {
				t.Fatalf("scoped value invalid after lifecycle rejection: %#v", values)
			}
			return nil
		},
	)
	if err != nil || !found {
		t.Fatalf("scoped lifecycle call = found %v, err %v", found, err)
	}
	if err := vm.DoString("after scoped lifecycle", `scoped_lifecycle_recovered = true`); err != nil {
		t.Fatalf("VM unusable after scoped lifecycle rejection: %v", err)
	}
}

func TestScriptEngineRestoresScopedCallAfterConsumerPanic(t *testing.T) {
	vm := newScriptEngine()
	initializeScriptEngine(t, vm)
	if err := vm.DoString("scoped panic setup", `
		probe = {}
		function probe.result()
			return {status = "alive"}
		end
	`); err != nil {
		t.Fatalf("scoped panic setup: %v", err)
	}

	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		_, _ = vm.CallModuleScoped(
			"probe",
			"result",
			1,
			nil,
			func([]script.Value) error { panic("scoped consumer panic") },
		)
	}()
	if panicValue == nil {
		t.Fatal("scoped consumer panic disappeared")
	}
	if err := vm.DoString("after scoped panic", `scoped_panic_recovered = true`); err != nil {
		t.Fatalf("VM unusable after scoped consumer panic: %v", err)
	}
	if err := vm.Init(); err != nil {
		t.Fatalf("lifecycle guard not restored after scoped consumer panic: %v", err)
	}
}

func TestScriptEngineReentrySharesWatchdog(t *testing.T) {
	vm := newScriptEngine()
	var nestedErr error
	vm.RegisterModule("probe", map[string]script.GoFunc{
		"reenter_spin": func(c *script.Call) error {
			_, found, err := vm.CallModule("probe", "spin", 0)
			if !found {
				return c.Errorf("nested spin function not found")
			}
			nestedErr = err
			return err
		},
		// Calling a host function keeps LuaJIT's loop interpreter-bound so its
		// asynchronously armed debug hook can interrupt it.
		"tick": func(*script.Call) error { return nil },
	}, nil)
	initializeScriptEngine(t, vm)

	loop := "while true do end"
	if vm.Backend() == "luajit" {
		loop = "while true do probe.tick() end"
	}
	if err := vm.DoString("watchdog setup", `
		function probe.spin()
			`+loop+`
		end
	`); err != nil {
		t.Fatalf("watchdog setup: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	vm.SetContext(ctx)
	started := time.Now()
	err := vm.DoString("nested watchdog", `probe.reenter_spin()`)
	elapsed := time.Since(started)
	vm.RemoveContext()

	if nestedErr == nil || err == nil {
		t.Fatalf("nested watchdog errors = nested %v, outer %v", nestedErr, err)
	}
	for _, got := range []error{nestedErr, err} {
		if strings.Contains(got.Error(), "state is executing") {
			t.Fatalf("watchdog reentry used idle State: %v", got)
		}
		if !strings.Contains(got.Error(), "interrupted") &&
			!strings.Contains(got.Error(), "deadline") {
			t.Fatalf("unexpected watchdog error: %v", got)
		}
	}
	if elapsed > 2*time.Second {
		t.Fatalf("nested watchdog took %v", elapsed)
	}
	if err := vm.DoString("idle after watchdog", `watchdog_recovered = true`); err != nil {
		t.Fatalf("idle Engine call after watchdog: %v", err)
	}
}

func TestScriptEngineReentrySharesWatchdogFromCoroutine(t *testing.T) {
	vm := newScriptEngine()
	var nestedErr error
	vm.RegisterModule("probe", map[string]script.GoFunc{
		"reenter_spin": func(c *script.Call) error {
			_, found, err := vm.CallModule("probe", "spin", 0)
			if !found {
				return c.Errorf("nested spin function not found")
			}
			nestedErr = err
			return err
		},
		"tick": func(*script.Call) error { return nil },
	}, nil)
	initializeScriptEngine(t, vm)

	loop := "while true do end"
	if vm.Backend() == "luajit" {
		loop = "while true do probe.tick() end"
	}
	if err := vm.DoString("coroutine watchdog setup", `
		function probe.spin()
			`+loop+`
		end
	`); err != nil {
		t.Fatalf("coroutine watchdog setup: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	vm.SetContext(ctx)
	started := time.Now()
	err := vm.DoString("coroutine nested watchdog", `
		local worker = coroutine.create(function()
			probe.reenter_spin()
		end)
		local ok, message = coroutine.resume(worker)
		assert(ok, message)
	`)
	elapsed := time.Since(started)
	vm.RemoveContext()

	if nestedErr == nil || err == nil {
		t.Fatalf("coroutine watchdog errors = nested %v, outer %v", nestedErr, err)
	}
	for _, got := range []error{nestedErr, err} {
		if strings.Contains(got.Error(), "state is executing") {
			t.Fatalf("coroutine watchdog reentry used idle State: %v", got)
		}
		if !strings.Contains(got.Error(), "interrupted") &&
			!strings.Contains(got.Error(), "deadline") {
			t.Fatalf("unexpected coroutine watchdog error: %v", got)
		}
	}
	if elapsed > 2*time.Second {
		t.Fatalf("coroutine nested watchdog took %v", elapsed)
	}
	if err := vm.DoString("idle after coroutine watchdog", `coroutine_watchdog_recovered = true`); err != nil {
		t.Fatalf("idle Engine call after coroutine watchdog: %v", err)
	}
}

func initializeScriptEngine(t *testing.T, vm script.Engine) {
	t.Helper()
	if err := vm.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vm.Close)
}
