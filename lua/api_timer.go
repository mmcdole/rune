package lua

import (
	"time"

	glua "github.com/yuin/gopher-lua"
)

// registerTimerFuncs registers rune._timer.* primitives.
func (e *Engine) registerTimerFuncs() {
	timerTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_timer", timerTable)

	// rune._timer.after(seconds, callback): One-shot timer, returns ID
	e.L.SetField(timerTable, "after", e.L.NewFunction(func(L *glua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2)

		id := e.timer.TimerAfter(toDuration(seconds))
		e.callbacks[id] = fn

		L.Push(glua.LNumber(id))
		return 1
	}))

	// rune._timer.every(seconds, callback): Repeating timer, returns ID
	e.L.SetField(timerTable, "every", e.L.NewFunction(func(L *glua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2)

		id := e.timer.TimerEvery(toDuration(seconds))
		e.callbacks[id] = fn

		L.Push(glua.LNumber(id))
		return 1
	}))

	// rune._timer.cancel(id): Stop a timer
	e.L.SetField(timerTable, "cancel", e.L.NewFunction(func(L *glua.LState) int {
		id := int(L.CheckNumber(1))
		if _, ok := e.callbacks[id]; ok {
			delete(e.callbacks, id)
			e.timer.TimerCancel(id)
		}
		return 0
	}))

	// rune._timer.cancel_all(): Stop all timers
	e.L.SetField(timerTable, "cancel_all", e.L.NewFunction(func(L *glua.LState) int {
		e.callbacks = make(map[int]*glua.LFunction)
		e.timer.TimerCancelAll()
		return 0
	}))
}

// toDuration converts Lua number seconds to Go duration
func toDuration(seconds glua.LNumber) time.Duration {
	return time.Duration(float64(seconds) * float64(time.Second))
}
