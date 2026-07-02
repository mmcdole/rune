package lua

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drake/rune/text"
	"github.com/drake/rune/ui"
	glua "github.com/yuin/gopher-lua"
)

// DefaultCallTimeout bounds each entry into the Lua VM. A script that
// exceeds it (e.g. an accidental infinite loop) is interrupted with an
// error instead of hanging the event loop forever.
const DefaultCallTimeout = 5 * time.Second

// Engine wraps gopher-lua and manages the VM lifecycle.
// It is a pure mechanism: it knows how to run Lua code and expose APIs.
// It does NOT know about core scripts, config dirs, or boot sequences.
type Engine struct {
	L         *glua.LState
	runeTable *glua.LTable
	host      Host

	// Cleared on reload to prevent stale Lua references
	pickerCallbacks map[string]*glua.LFunction
	pickerNextID    int

	// Layout config, marshaled from rune.ui.layout calls
	barLayout ui.LayoutConfig

	// Watchdog
	CallTimeout time.Duration      // Time budget per Lua entry; see DefaultCallTimeout
	inLua       bool               // True while inside a guarded Lua call (re-entrancy)
	guardCancel context.CancelFunc // Cancels the active watchdog context

	// True once the user has been warned that rune.hooks.call is
	// missing and the client is degraded to raw pass-through.
	hooksBrokenReported bool

	// True while dispatching the "error" event, so failures inside
	// error handlers print directly instead of recursing.
	reportingError bool
}

// NewEngine creates an Engine with a Host interface.
func NewEngine(host Host) *Engine {
	return &Engine{
		host:            host,
		pickerCallbacks: make(map[string]*glua.LFunction),
		barLayout: ui.LayoutConfig{
			Bottom: []ui.LayoutEntry{{Name: "input"}, {Name: "status"}},
		},
		CallTimeout: DefaultCallTimeout,
	}
}

// guard runs fn under the watchdog: a deadline context is attached to
// the VM so runaway scripts are interrupted instead of hanging the
// event loop. Nested entries (Go APIs called from Lua that re-enter
// the engine, e.g. rune._load) run under the outermost deadline.
func (e *Engine) guard(fn func() error) error {
	if e.inLua {
		return fn()
	}
	e.inLua = true
	ctx, cancel := context.WithTimeout(context.Background(), e.CallTimeout)
	e.guardCancel = cancel
	e.L.SetContext(ctx)
	defer func() {
		e.L.RemoveContext()
		e.guardCancel()
		e.guardCancel = nil
		e.inLua = false
	}()

	err := fn()
	// The active context may have been replaced by pauseWatchdog, so
	// consult the VM's current context rather than the original.
	if lctx := e.L.Context(); err != nil && lctx != nil && lctx.Err() != nil {
		return fmt.Errorf("script interrupted after %v (runaway loop?): %w", e.CallTimeout, err)
	}
	return err
}

// pauseWatchdog runs fn with the watchdog deadline detached, then arms
// a fresh full deadline for the remainder of the Lua entry. Use it for
// host calls that legitimately block on the user (e.g. an external
// $EDITOR): time spent there is not runaway script time, and without
// this the expired deadline would kill the calling handler the moment
// it resumed.
func (e *Engine) pauseWatchdog(fn func()) {
	if !e.inLua {
		fn()
		return
	}
	e.L.RemoveContext()
	e.guardCancel()

	fn()

	ctx, cancel := context.WithTimeout(context.Background(), e.CallTimeout)
	e.guardCancel = cancel
	e.L.SetContext(ctx)
}

// Init initializes (or re-initializes) the Lua VM with fresh state.
// It registers the API but does NOT load any scripts - that's the caller's job.
func (e *Engine) Init() error {
	if e.L != nil {
		e.L.Close()
	}

	e.L = glua.NewState()

	e.host.TimerCancelAll()

	e.pickerCallbacks = make(map[string]*glua.LFunction)
	e.pickerNextID = 0

	e.barLayout = ui.LayoutConfig{
		Bottom: []ui.LayoutEntry{{Name: "input"}, {Name: "status"}},
	}
	e.hooksBrokenReported = false

	registerLineType(e.L)
	e.registerAPIs()

	return nil
}

// Close cleans up the Lua state.
func (e *Engine) Close() {
	e.host.TimerCancelAll()
	if e.L != nil {
		e.L.Close()
		e.L = nil
	}
}

// OnTimer handles wake-up calls from Session by dispatching to the
// Lua timer module, which owns the id -> callback mapping. Ids from a
// previous VM generation (or cancelled mid-flight) are ignored there.
func (e *Engine) OnTimer(id int) {
	if e.L == nil {
		return
	}

	fire, ok := e.getRuneFunc("timer", "_fire")
	if !ok {
		return // Timer module unavailable (core failed to load)
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      fire,
			NRet:    0,
			Protect: true,
		}, glua.LNumber(id))
	}); err != nil {
		e.reportError("timer callback", err)
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
	if err := e.guard(func() error { return e.L.PCall(1, 0, nil) }); err != nil {
		e.reportError("picker callback", err)
	}
}

// CancelPickerCallback removes a picker callback without executing it.
func (e *Engine) CancelPickerCallback(id string) {
	delete(e.pickerCallbacks, id)
}

// SetConfigDir exposes the config directory to scripts as
// rune.config_dir. Set directly on the table rather than via generated
// Lua source, so arbitrary path characters cannot break or inject code.
func (e *Engine) SetConfigDir(dir string) {
	e.L.SetField(e.runeTable, "config_dir", glua.LString(dir))
}

// DoString executes a raw string of Lua code.
// The name parameter is used for stack traces.
func (e *Engine) DoString(name, code string) error {
	fn, err := e.L.Load(strings.NewReader(code), name)
	if err != nil {
		return err
	}
	e.L.Push(fn)
	return e.guard(func() error { return e.L.PCall(0, 0, nil) })
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

	err = e.guard(func() error { return e.L.DoFile(absPath) })

	e.L.SetField(pkg, "path", glua.LString(oldPath))

	return err
}

// OnInput handles user typing.
func (e *Engine) OnInput(text string) bool {
	hooksCall, ok := e.getHooksCall()
	if !ok {
		e.reportHooksBroken()
		// Degraded mode: keep the escape hatches working, pass
		// everything else to the server as a plain telnet client.
		switch text {
		case "/quit":
			e.host.Quit()
		case "/reload":
			e.host.Reload()
		default:
			e.host.Send(text)
		}
		return true
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      hooksCall,
			NRet:    1,
			Protect: true,
		}, glua.LString("input"), glua.LString(text))
	}); err != nil {
		e.reportError("input dispatch", err)
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
	hooksCall, ok := e.getHooksCall()
	if !ok {
		e.reportHooksBroken()
		return line.Raw, true
	}

	lineUD := newLine(e.L, line)

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      hooksCall,
			NRet:    2,
			Protect: true,
		}, glua.LString("output"), lineUD)
	}); err != nil {
		e.reportError("output dispatch", err)
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
	hooksCall, ok := e.getHooksCall()
	if !ok {
		e.reportHooksBroken()
		return line.Raw
	}

	lineUD := newLine(e.L, line)

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      hooksCall,
			NRet:    2,
			Protect: true,
		}, glua.LString("prompt"), lineUD)
	}); err != nil {
		e.reportError("prompt dispatch", err)
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
	hooksCall, ok := e.getHooksCall()
	if !ok {
		e.reportHooksBroken()
		// Errors must never disappear, even with hooks broken.
		if event == "error" {
			e.host.Print(text.Red("[Error] " + strings.Join(args, " ")))
		}
		return
	}

	luaArgs := make([]glua.LValue, len(args)+1)
	luaArgs[0] = glua.LString(event)
	for i, arg := range args {
		luaArgs[i+1] = glua.LString(arg)
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      hooksCall,
			NRet:    0,
			Protect: true,
		}, luaArgs...)
	}); err != nil {
		e.reportError("'"+event+"' hook", err)
	}
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
	e.registerPickerFuncs()
	e.registerHistoryFuncs()
	e.registerInputFuncs()
}

// getRuneFunc returns rune.<table>.<field> if it is a function.
// Returns false when the module is unavailable - because a core
// script failed to load, or a user script clobbered the table.
func (e *Engine) getRuneFunc(table, field string) (glua.LValue, bool) {
	tbl, ok := e.L.GetField(e.runeTable, table).(*glua.LTable)
	if !ok {
		return glua.LNil, false
	}
	fn := e.L.GetField(tbl, field)
	if fn.Type() != glua.LTFunction {
		return glua.LNil, false
	}
	return fn, true
}

// getHooksCall returns rune.hooks.call, the Lua-side dispatcher for
// all events.
func (e *Engine) getHooksCall() (glua.LValue, bool) {
	return e.getRuneFunc("hooks", "call")
}

// reportError surfaces an engine-level error to the user through the
// Lua "error" hook. Failures inside the error hook itself fall back to
// direct printing rather than recursing.
func (e *Engine) reportError(source string, err error) {
	msg := source + ": " + err.Error()
	if e.reportingError {
		e.host.Print(text.Red("[Error] " + msg))
		return
	}
	e.reportingError = true
	defer func() { e.reportingError = false }()
	e.CallHook("error", msg)
}

// reportHooksBroken warns the user, once per VM generation, that the
// hook system is unavailable and the client is running degraded.
func (e *Engine) reportHooksBroken() {
	if e.hooksBrokenReported {
		return
	}
	e.hooksBrokenReported = true
	e.host.Print(text.Red("[System] rune.hooks.call is unavailable - scripting disabled. " +
		"Input and output pass through raw; /reload and /quit still work. " +
		"Fix your scripts and /reload."))
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
