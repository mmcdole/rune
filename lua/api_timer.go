package lua

import (
	"time"

	glua "github.com/yuin/gopher-lua"
)

// registerTimerFuncs registers rune._timer.* primitives.
//
// Go only schedules wake-ups and returns ids; the Lua timer module
// (40_timers.lua) owns the id -> callback mapping and all dispatch,
// so callback state lives in exactly one place and dies with the VM
// on reload.
func (e *Engine) registerTimerFuncs() {
	timerTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_timer", timerTable)

	// rune._timer.after(seconds): One-shot wake-up, returns ID
	e.L.SetField(timerTable, "after", e.L.NewFunction(func(L *glua.LState) int {
		seconds := L.CheckNumber(1)
		L.Push(glua.LNumber(e.host.TimerAfter(toDuration(seconds))))
		return 1
	}))

	// rune._timer.every(seconds): Repeating wake-up, returns ID
	e.L.SetField(timerTable, "every", e.L.NewFunction(func(L *glua.LState) int {
		seconds := L.CheckNumber(1)
		L.Push(glua.LNumber(e.host.TimerEvery(toDuration(seconds))))
		return 1
	}))

	// rune._timer.cancel(id): Stop a scheduled wake-up
	e.L.SetField(timerTable, "cancel", e.L.NewFunction(func(L *glua.LState) int {
		e.host.TimerCancel(int(L.CheckNumber(1)))
		return 0
	}))

	// rune._timer.cancel_all(): Stop all scheduled wake-ups
	e.L.SetField(timerTable, "cancel_all", e.L.NewFunction(func(L *glua.LState) int {
		e.host.TimerCancelAll()
		return 0
	}))
}

// toDuration converts Lua number seconds to Go duration
func toDuration(seconds glua.LNumber) time.Duration {
	return time.Duration(float64(seconds) * float64(time.Second))
}
