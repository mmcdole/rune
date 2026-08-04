package lua

import (
	"time"

	"github.com/mmcdole/rune/script"
)

// registerTimerFuncs registers rune._timer.* primitives.
//
// Go only schedules wake-ups and returns ids; the Lua timer module
// (40_timers.lua) owns the id -> callback mapping and all dispatch,
// so callback state lives in exactly one place and dies with the VM
// on reload.
func (e *Engine) registerTimerFuncs() {
	e.vm.RegisterModule("rune._timer", map[string]script.GoFunc{
		// rune._timer.after(seconds): One-shot wake-up, returns ID
		"after": func(c *script.Call) error {
			id := e.host.TimerAfter(toDuration(c.Num(1)))
			c.Return(id)
			return nil
		},

		// rune._timer.every(seconds): Repeating wake-up, returns ID
		"every": func(c *script.Call) error {
			id := e.host.TimerEvery(toDuration(c.Num(1)))
			c.Return(id)
			return nil
		},

		// rune._timer.cancel(id): Stop a scheduled wake-up
		"cancel": func(c *script.Call) error {
			e.host.TimerCancel(c.Int(1))
			return nil
		},

		// rune._timer.cancel_all(): Stop all scheduled wake-ups
		"cancel_all": func(c *script.Call) error {
			e.host.TimerCancelAll()
			return nil
		},
	}, nil)
}

// toDuration converts script number seconds to Go duration
func toDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
