package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drake/rune/text"
	glua "github.com/yuin/gopher-lua"
)

// Engine wraps gopher-lua and manages the VM lifecycle.
// It is a pure mechanism: it knows how to run Lua code and expose APIs.
// It does NOT know about core scripts, config dirs, or boot sequences.
type Engine struct {
	L         *glua.LState
	runeTable *glua.LTable
	host      Host
	callbacks map[int]*glua.LFunction

	// Cleared on reload to prevent stale Lua references
	pickerCallbacks map[string]*glua.LFunction
	pickerNextID    int

	bars  *barRegistry
	binds *bindRegistry
}

// NewEngine creates an Engine with a Host interface.
func NewEngine(host Host) *Engine {
	return &Engine{
		host:            host,
		callbacks:       make(map[int]*glua.LFunction),
		pickerCallbacks: make(map[string]*glua.LFunction),
		bars:            newBarRegistry(),
		binds:           newBindRegistry(),
	}
}

// Init initializes (or re-initializes) the Lua VM with fresh state.
// It registers the API but does NOT load any scripts - that's the caller's job.
func (e *Engine) Init() error {
	if e.L != nil {
		e.L.Close()
	}

	e.L = glua.NewState()

	e.host.TimerCancelAll()
	e.callbacks = make(map[int]*glua.LFunction)

	e.pickerCallbacks = make(map[string]*glua.LFunction)
	e.pickerNextID = 0

	e.bars = newBarRegistry()
	e.binds = newBindRegistry()

	registerLineType(e.L)
	e.registerAPIs()

	return nil
}

// Close cleans up the Lua state.
func (e *Engine) Close() {
	e.host.TimerCancelAll()
	e.callbacks = nil
	if e.L != nil {
		e.L.Close()
		e.L = nil
	}
}

// OnTimer handles wake-up calls from Session.
func (e *Engine) OnTimer(id int, repeating bool) {
	if e.L == nil {
		return
	}

	fn, ok := e.callbacks[id]
	if !ok {
		return // Cancelled, or belonged to previous Engine instance
	}

	e.L.Push(fn)
	if err := e.L.PCall(0, 0, nil); err != nil {
		e.CallHook("error", "timer: "+err.Error())
	}

	if !repeating {
		delete(e.callbacks, id)
	}
}

// RegisterPickerCallback stores a Lua function for later execution when the
// picker selection is made. Returns a unique callback ID.
func (e *Engine) RegisterPickerCallback(fn *glua.LFunction) string {
	e.pickerNextID++
	id := fmt.Sprintf("p%d", e.pickerNextID)
	e.pickerCallbacks[id] = fn
	return id
}

// ExecutePickerCallback runs the Lua callback for a picker selection.
// Safe to call after reload - stale callbacks are silently ignored.
func (e *Engine) ExecutePickerCallback(id string, value string) {
	fn, ok := e.pickerCallbacks[id]
	if !ok || e.L == nil {
		return
	}
	delete(e.pickerCallbacks, id)
	e.L.Push(fn)
	e.L.Push(glua.LString(value))
	if err := e.L.PCall(1, 0, nil); err != nil {
		e.CallHook("error", "picker callback: "+err.Error())
	}
}

// CancelPickerCallback removes a picker callback without executing it.
func (e *Engine) CancelPickerCallback(id string) {
	delete(e.pickerCallbacks, id)
}

// DoString executes a raw string of Lua code.
// The name parameter is used for stack traces.
func (e *Engine) DoString(name, code string) error {
	fn, err := e.L.Load(strings.NewReader(code), name)
	if err != nil {
		return err
	}
	e.L.Push(fn)
	return e.L.PCall(0, 0, nil)
}

// DoFile executes a Lua file from the filesystem.
// Temporarily adjusts package.path to allow local requires.
func (e *Engine) DoFile(path string) error {
	path = expandTilde(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(absPath)

	pkg := e.L.GetGlobal("package").(*glua.LTable)
	oldPath := e.L.GetField(pkg, "path").String()
	newPath := dir + "/?.lua;" + oldPath
	e.L.SetField(pkg, "path", glua.LString(newPath))

	err = e.L.DoFile(absPath)

	e.L.SetField(pkg, "path", glua.LString(oldPath))

	return err
}

// OnInput handles user typing.
func (e *Engine) OnInput(text string) bool {
	if err := e.L.CallByParam(glua.P{
		Fn:      e.getHooksCall(),
		NRet:    1,
		Protect: true,
	}, glua.LString("input"), glua.LString(text)); err != nil {
		return false
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	if ret == glua.LFalse {
		return false
	}
	return true
}

// OnOutput handles server text.
func (e *Engine) OnOutput(line text.Line) (string, bool) {
	lineUD := newLine(e.L, line)

	if err := e.L.CallByParam(glua.P{
		Fn:      e.getHooksCall(),
		NRet:    2,
		Protect: true,
	}, glua.LString("output"), lineUD); err != nil {
		return line.Raw, true
	}

	show := e.L.Get(-1)
	modified := e.L.Get(-2)
	e.L.Pop(2)

	if show == glua.LFalse {
		return "", false
	}
	return modified.String(), true
}

// OnPrompt handles server prompts.
func (e *Engine) OnPrompt(line text.Line) string {
	lineUD := newLine(e.L, line)

	if err := e.L.CallByParam(glua.P{
		Fn:      e.getHooksCall(),
		NRet:    2,
		Protect: true,
	}, glua.LString("prompt"), lineUD); err != nil {
		return line.Raw
	}

	show := e.L.Get(-1)
	modified := e.L.Get(-2)
	e.L.Pop(2)

	if show == glua.LFalse {
		return ""
	}
	return modified.String()
}

// CallHook calls a hook event with string arguments.
func (e *Engine) CallHook(event string, args ...string) {
	luaArgs := make([]glua.LValue, len(args)+1)
	luaArgs[0] = glua.LString(event)
	for i, arg := range args {
		luaArgs[i+1] = glua.LString(arg)
	}

	e.L.CallByParam(glua.P{
		Fn:      e.getHooksCall(),
		NRet:    0,
		Protect: true,
	}, luaArgs...)
}

func (e *Engine) registerAPIs() {
	e.runeTable = e.L.NewTable()
	e.L.SetGlobal("rune", e.runeTable)

	e.registerCoreFuncs()
	e.registerTimerFuncs()
	e.registerRegexFuncs()
	e.registerUIFuncs()
	e.registerStateFuncs()
	e.registerBarFuncs()
	e.registerBindFuncs()
	e.registerPickerFuncs()
	e.registerHistoryFuncs()
	e.registerInputFuncs()
}

func (e *Engine) getHooksCall() glua.LValue {
	hooksTable := e.L.GetField(e.runeTable, "hooks").(*glua.LTable)
	return e.L.GetField(hooksTable, "call")
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
