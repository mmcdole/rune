package lua

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"
	glua "github.com/yuin/gopher-lua"
)

// Engine wraps gopher-lua and manages the VM lifecycle.
// It is a pure mechanism: it knows how to run Lua code and expose APIs.
// It does NOT know about core scripts, config dirs, or boot sequences.
type Engine struct {
	L          *glua.LState
	regexCache *lru.Cache[string, *regexp.Regexp]

	// Cached table reference
	runeTable *glua.LTable

	// Host interface for communication with the rest of the system
	host Host

	// Timer callbacks - Engine owns callbacks, Timer service owns IDs and scheduling
	callbacks map[int]*glua.LFunction
}

// NewEngine creates an Engine with the given Host.
func NewEngine(host Host) *Engine {
	cache, _ := lru.New[string, *regexp.Regexp](100)
	return &Engine{
		regexCache: cache,
		host:       host,
		callbacks:  make(map[int]*glua.LFunction),
	}
}

// --- Lifecycle ---

// Init initializes (or re-initializes) the Lua VM with fresh state.
// It registers the API but does NOT load any scripts - that's the caller's job.
func (e *Engine) Init() error {
	// Close old Lua state if it exists
	if e.L != nil {
		e.L.Close()
	}

	// Create fresh Lua state
	e.L = glua.NewState()

	// Reset regex cache
	cache, _ := lru.New[string, *regexp.Regexp](100)
	e.regexCache = cache

	// Cancel all pending timers and clear callback map
	e.host.TimerCancelAll()
	e.callbacks = make(map[int]*glua.LFunction)

	// Register custom types
	registerLineType(e.L)

	// Register API functions
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
// This is the single entry point for all timer callback execution.
func (e *Engine) OnTimer(id int, repeating bool) {
	if e.L == nil {
		return
	}

	fn, ok := e.callbacks[id]
	if !ok {
		return // Cancelled, or belonged to previous Engine instance
	}

	// Execute callback with protected call
	e.L.Push(fn)
	if err := e.L.PCall(0, 0, nil); err != nil {
		e.CallHook("error", "timer: "+err.Error())
	}

	// Clean up one-shot timer callbacks
	if !repeating {
		delete(e.callbacks, id)
	}
}

// --- Execution Primitives (Mechanism) ---

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
// It temporarily adjusts package.path to allow local requires.
func (e *Engine) DoFile(path string) error {
	path = expandTilde(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(absPath)

	// Temporarily prepend script's directory to package.path
	pkg := e.L.GetGlobal("package").(*glua.LTable)
	oldPath := e.L.GetField(pkg, "path").String()
	newPath := dir + "/?.lua;" + oldPath
	e.L.SetField(pkg, "path", glua.LString(newPath))

	err = e.L.DoFile(absPath)

	// Restore original path
	e.L.SetField(pkg, "path", glua.LString(oldPath))

	return err
}

// --- Event Handlers ---

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
func (e *Engine) OnOutput(text string) (string, bool) {
	clean := stripAnsi(text)
	lineUD := newLine(e.L, text, clean)

	if err := e.L.CallByParam(glua.P{
		Fn:      e.getHooksCall(),
		NRet:    2,
		Protect: true,
	}, glua.LString("output"), lineUD); err != nil {
		return text, true
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
func (e *Engine) OnPrompt(text string) string {
	clean := stripAnsi(text)
	lineUD := newLine(e.L, text, clean)

	if err := e.L.CallByParam(glua.P{
		Fn:      e.getHooksCall(),
		NRet:    2,
		Protect: true,
	}, glua.LString("prompt"), lineUD); err != nil {
		return text
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

// ExecuteCallback runs a callback function.
func (e *Engine) ExecuteCallback(cb func()) {
	if cb != nil {
		cb()
	}
}

// --- API Registration ---

func (e *Engine) registerAPIs() {
	e.runeTable = e.L.NewTable()
	e.L.SetGlobal("rune", e.runeTable)

	e.registerCoreFuncs()
	e.registerTimerFuncs()
	e.registerRegexFuncs()
	e.registerUIFuncs()
}

// getHooksCall returns the rune.hooks.call function.
func (e *Engine) getHooksCall() glua.LValue {
	hooksTable := e.L.GetField(e.runeTable, "hooks").(*glua.LTable)
	return e.L.GetField(hooksTable, "call")
}

// --- Private Helpers ---

// stripAnsi removes ANSI escape codes from a string.
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// expandTilde expands ~ to home directory.
func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
