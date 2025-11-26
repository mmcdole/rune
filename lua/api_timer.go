package lua

import (
	"time"

	glua "github.com/yuin/gopher-lua"
)

// registerTimerFuncs registers internal rune._timer.* primitives (wrapped by Lua)
func (e *Engine) registerTimerFuncs() {
	timerTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_timer", timerTable)

	// rune._timer.after(seconds, callback): Schedule delayed callback
	e.L.SetField(timerTable, "after", e.L.NewFunction(func(L *glua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2)

		time.AfterFunc(toDuration(seconds), func() {
			e.host.SendTimerEvent(func() {
				L.Push(fn)
				L.PCall(0, 0, nil)
			})
		})
		return 0
	}))

	// rune._timer.every(seconds, callback): Schedule repeating callback, returns timer ID
	e.L.SetField(timerTable, "every", e.L.NewFunction(func(L *glua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2)

		e.timerMu.Lock()
		e.timerID++
		id := e.timerID
		ticker := time.NewTicker(toDuration(seconds))
		done := make(chan struct{})
		e.timers[id] = &timerEntry{ticker: ticker, done: done}
		e.timerMu.Unlock()

		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					e.host.SendTimerEvent(func() {
						L.Push(fn)
						L.PCall(0, 0, nil)
					})
				}
			}
		}()

		L.Push(glua.LNumber(id))
		return 1
	}))

	// rune._timer.cancel(id): Cancel a repeating timer
	e.L.SetField(timerTable, "cancel", e.L.NewFunction(func(L *glua.LState) int {
		id := int(L.CheckNumber(1))
		e.cancelTimer(id)
		return 0
	}))

	// rune._timer.cancel_all(): Cancel all repeating timers
	e.L.SetField(timerTable, "cancel_all", e.L.NewFunction(func(L *glua.LState) int {
		e.CancelAllTimers()
		return 0
	}))
}

// cancelTimer cancels a single timer by ID
func (e *Engine) cancelTimer(id int) {
	e.timerMu.Lock()
	if entry, ok := e.timers[id]; ok {
		close(entry.done)
		entry.ticker.Stop()
		delete(e.timers, id)
	}
	e.timerMu.Unlock()
}

// toDuration converts Lua number seconds to Go duration
func toDuration(seconds glua.LNumber) time.Duration {
	return time.Duration(float64(seconds) * float64(time.Second))
}
