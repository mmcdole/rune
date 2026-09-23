package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/script"
)

func TestScriptEngineRejectsUnsupportedReturns(t *testing.T) {
	bad := make(chan int)
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"value", bad},
		{"tree_scalar", script.Tree{V: bad}},
		{"tree_slice", script.Tree{V: []any{"valid", map[string]any{"bad": bad}}}},
		{"tree_map", script.Tree{V: map[string]any{"items": []any{"valid", bad}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vm := newScriptEngine()
			vm.RegisterModule("probe", map[string]script.GoFunc{
				"bad": func(c *script.Call) error {
					// Fail after a valid result has already been converted.
					c.Return("first", tc.value, "last")
					return nil
				},
				"good": func(c *script.Call) error {
					c.Return(c.Str(1), script.Tree{V: []any{"alive"}}, 42)
					return nil
				},
			}, nil)
			initializeScriptEngine(t, vm)

			if err := vm.DoString("caught return error", `
				function probe.check()
					local ok, message, extra = pcall(probe.bad, "argument")
					assert(not ok, "invalid return succeeded")
					assert(type(message) == "string", "missing error message")
					assert(string.find(message, "chan int", 1, true), message)
					assert(extra == nil, "partial result escaped")
					local first, tree, last = probe.good("intact")
					assert(first == "intact" and tree[1] == "alive" and last == 42)
				end
				probe.check()
				local worker = coroutine.create(probe.check)
				local ok, message = coroutine.resume(worker)
				assert(ok, message)
			`); err != nil {
				t.Fatalf("pcall did not handle conversion failure: %v", err)
			}

			if err := vm.DoString("uncaught return error", `probe.bad()`); err == nil ||
				!strings.Contains(err.Error(), "chan int") {
				t.Fatalf("uncaught conversion error = %v", err)
			}
			if err := vm.DoString("after return error", `probe.check()`); err != nil {
				t.Fatalf("VM unusable after conversion failure: %v", err)
			}
		})
	}
}

func TestScriptEngineRejectsUnsupportedArguments(t *testing.T) {
	for _, method := range []string{"module", "pinned"} {
		t.Run(method, func(t *testing.T) {
			vm := newScriptEngine()
			var target script.FuncRef
			vm.RegisterModule("probe", map[string]script.GoFunc{
				"pin": func(c *script.Call) error {
					target = c.PinFunc(1)
					return nil
				},
			}, nil)
			initializeScriptEngine(t, vm)
			if err := vm.DoString("argument conversion setup", `
				calls = 0
				function probe.target(first, second, last)
					calls = calls + 1
					return first, second, last
				end
				probe.pin(probe.target)
			`); err != nil {
				t.Fatal(err)
			}
			defer target.Release()
			invoke := func(args ...any) ([]script.Result, error) {
				if method == "pinned" {
					return vm.Call(target, 3, args...)
				}
				results, found, err := vm.CallModule("probe", "target", 3, args...)
				if !found {
					t.Fatal("target function missing")
				}
				return results, err
			}
			bad := make(chan int)
			for _, value := range []any{bad, script.Tree{V: []any{"valid", bad}}} {
				if _, err := invoke("first", value, "last"); err == nil ||
					!strings.Contains(err.Error(), "chan int") {
					t.Fatalf("argument conversion error = %v", err)
				}
			}
			if err := vm.DoString("after argument errors", `assert(calls == 0)`); err != nil {
				t.Fatalf("invalid arguments reached target: %v", err)
			}
			results, err := invoke("first", "second", "last")
			if err != nil || len(results) != 3 {
				t.Fatalf("call after argument errors: results %v, error %v", results, err)
			}
			for i, want := range []string{"first", "second", "last"} {
				if results[i].String() != want {
					t.Errorf("result %d = %q, want %q", i, results[i].String(), want)
				}
			}
		})
	}
}
