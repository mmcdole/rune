package engine

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/drake/rune/mud"
	lua "github.com/yuin/gopher-lua"
)

// timerEntry holds a timer and its done channel for clean shutdown
type timerEntry struct {
	ticker *time.Ticker
	done   chan struct{}
}

// LuaEngine implements the ScriptEngine interface
type LuaEngine struct {
	L          *lua.LState
	regexCache *lru.Cache[string, *regexp.Regexp]
	timers     map[int]*timerEntry
	timerID    int
	timerMu    sync.Mutex

	// Cached table reference
	runeTable *lua.LTable

	// Channel references for reload
	events  chan<- mud.Event
	uplink  chan<- string
	display chan<- string
}

// NewLuaEngine initializes a Lua VM with regex caching and timer management.
func NewLuaEngine(events chan<- mud.Event, uplink chan<- string, display chan<- string) *LuaEngine {
	cache, _ := lru.New[string, *regexp.Regexp](100)
	return &LuaEngine{
		regexCache: cache,
		timers:     make(map[int]*timerEntry),
		events:     events,
		uplink:     uplink,
		display:    display,
	}
}

// --- Setup/Configuration ---

// SetConfigDir sets the rune.config_dir variable for user scripts
func (e *LuaEngine) SetConfigDir(dir string) {
	e.L.SetField(e.runeTable, "config_dir", lua.LString(dir))
}

// --- Script Loading ---

// LoadEmbeddedCore loads core scripts from embedded filesystem
func (e *LuaEngine) LoadEmbeddedCore(scripts embed.FS) error {
	// Read all files from core directory
	entries, err := fs.ReadDir(scripts, "core")
	if err != nil {
		return fmt.Errorf("reading core scripts: %w", err)
	}

	// Sort entries to ensure consistent load order (00_, 10_, 20_, etc.)
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	// Load each file
	for _, file := range files {
		content, err := scripts.ReadFile("core/" + file)
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}
		if err := e.L.DoString(string(content)); err != nil {
			return fmt.Errorf("executing %s: %w", file, err)
		}
	}

	return nil
}

// LoadUserScripts loads user scripts from filesystem paths
func (e *LuaEngine) LoadUserScripts(paths []string) error {
	for _, path := range paths {
		// Expand ~ to home directory
		path = expandTilde(path)

		// Get absolute path and directory
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", path, err)
		}
		dir := filepath.Dir(absPath)

		// Read current package.path
		pkg := e.L.GetGlobal("package").(*lua.LTable)
		oldPath := e.L.GetField(pkg, "path").String()

		// Temporarily prepend script's directory to package.path
		newPath := dir + "/?.lua;" + oldPath
		e.L.SetField(pkg, "path", lua.LString(newPath))

		// Load the script
		content, err := os.ReadFile(absPath)
		if err != nil {
			e.L.SetField(pkg, "path", lua.LString(oldPath))
			return fmt.Errorf("reading %s: %w", absPath, err)
		}

		if err := e.L.DoString(string(content)); err != nil {
			e.L.SetField(pkg, "path", lua.LString(oldPath))
			return fmt.Errorf("executing %s: %w", absPath, err)
		}

		// Restore original package.path
		e.L.SetField(pkg, "path", lua.LString(oldPath))
	}
	return nil
}

// --- Event Handlers ---

// OnInput handles user typing
func (e *LuaEngine) OnInput(text string) bool {
	if err := e.L.CallByParam(lua.P{
		Fn:      e.getHooksCall(),
		NRet:    1,
		Protect: true,
	}, lua.LString("input"), lua.LString(text)); err != nil {
		return false
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	// false means input was consumed/stopped
	if ret == lua.LFalse {
		return false
	}
	return true
}

// OnOutput handles server text
func (e *LuaEngine) OnOutput(text string) (string, bool) {
	if err := e.L.CallByParam(lua.P{
		Fn:      e.getHooksCall(),
		NRet:    2,
		Protect: true,
	}, lua.LString("output"), lua.LString(text)); err != nil {
		return text, true
	}

	// hooks.call returns (modified_text, show)
	show := e.L.Get(-1)
	modified := e.L.Get(-2)
	e.L.Pop(2)

	if show == lua.LFalse {
		return "", false
	}

	return modified.String(), true
}

// OnPrompt handles server prompts
func (e *LuaEngine) OnPrompt(text string) string {
	if err := e.L.CallByParam(lua.P{
		Fn:      e.getHooksCall(),
		NRet:    2,
		Protect: true,
	}, lua.LString("prompt"), lua.LString(text)); err != nil {
		return text
	}

	// hooks.call returns (modified_text, show)
	show := e.L.Get(-1)
	modified := e.L.Get(-2)
	e.L.Pop(2)

	if show == lua.LFalse {
		return ""
	}

	return modified.String()
}

// CallHook calls a hook event with string arguments
func (e *LuaEngine) CallHook(event string, args ...string) {
	// Build args: event name + string args
	luaArgs := make([]lua.LValue, len(args)+1)
	luaArgs[0] = lua.LString(event)
	for i, arg := range args {
		luaArgs[i+1] = lua.LString(arg)
	}

	e.L.CallByParam(lua.P{
		Fn:      e.getHooksCall(),
		NRet:    0,
		Protect: true,
	}, luaArgs...)
}

// ExecuteCallback runs a stored Lua function
func (e *LuaEngine) ExecuteCallback(cb func()) {
	if cb != nil {
		cb()
	}
}

// --- Timer Management ---

// CancelAllTimers stops all repeating timers
func (e *LuaEngine) CancelAllTimers() {
	e.timerMu.Lock()
	for id, entry := range e.timers {
		close(entry.done)
		entry.ticker.Stop()
		delete(e.timers, id)
	}
	e.timerMu.Unlock()
}

// --- Lifecycle ---

// ClearRequireCache clears the Lua require cache so modules reload fresh
func (e *LuaEngine) ClearRequireCache() {
	pkg := e.L.GetGlobal("package").(*lua.LTable)
	loaded := e.L.GetField(pkg, "loaded").(*lua.LTable)
	// Clear all loaded modules
	loaded.ForEach(func(key, value lua.LValue) {
		e.L.SetField(loaded, key.String(), lua.LNil)
	})
}

// InitState initializes (or re-initializes) the Lua VM with fresh state
func (e *LuaEngine) InitState(coreScripts embed.FS, configDir string) error {
	// Cancel all timers
	e.CancelAllTimers()

	// Close old Lua state if it exists
	if e.L != nil {
		e.L.Close()
	}

	// Create fresh Lua state
	e.L = lua.NewState()
	cache, _ := lru.New[string, *regexp.Regexp](100)
	e.regexCache = cache
	e.timers = make(map[int]*timerEntry)
	e.timerID = 0

	// Bind host functions to the new state
	e.registerHostFuncs()

	// Set config dir
	e.SetConfigDir(configDir)

	// Load core scripts
	if err := e.LoadEmbeddedCore(coreScripts); err != nil {
		return err
	}

	// Fire ready hook after core but before user scripts
	e.CallHook("ready")

	// Load init.lua
	initPath := filepath.Join(configDir, "init.lua")
	if _, err := os.Stat(initPath); err == nil {
		if err := e.LoadUserScripts([]string{initPath}); err != nil {
			return err
		}
	}
	return nil
}

// Close cleans up the Lua state
func (e *LuaEngine) Close() {
	e.CancelAllTimers()
	e.L.Close()
}

// --- Private Helpers ---

// registerHostFuncs does the actual binding using stored channel references
func (e *LuaEngine) registerHostFuncs() {
	e.runeTable = e.L.NewTable()
	e.L.SetGlobal("rune", e.runeTable)

	e.registerCoreFuncs()
	e.registerTimerFuncs()
	e.registerRegexFuncs()
}

// registerCoreFuncs registers core rune.* functions
func (e *LuaEngine) registerCoreFuncs() {
	// rune.send_raw(text): Bypasses alias processing, writes directly to socket
	e.L.SetField(e.runeTable, "send_raw", e.L.NewFunction(func(L *lua.LState) int {
		cmd := L.CheckString(1)
		e.uplink <- cmd
		return 0
	}))

	// rune.print(text): Outputs text to the local display
	e.L.SetField(e.runeTable, "print", e.L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		e.display <- msg
		return 0
	}))

	// rune.quit(): Exit the client
	e.L.SetField(e.runeTable, "quit", e.L.NewFunction(func(L *lua.LState) int {
		e.events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionQuit},
		}
		return 0
	}))

	// rune.connect(address): Connect to server
	e.L.SetField(e.runeTable, "connect", e.L.NewFunction(func(L *lua.LState) int {
		addr := L.CheckString(1)
		e.events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionConnect, Address: addr},
		}
		return 0
	}))

	// rune.disconnect(): Disconnect from server
	e.L.SetField(e.runeTable, "disconnect", e.L.NewFunction(func(L *lua.LState) int {
		e.events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionDisconnect},
		}
		return 0
	}))

	// rune.reload(): Reload all scripts
	e.L.SetField(e.runeTable, "reload", e.L.NewFunction(func(L *lua.LState) int {
		e.events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionReload},
		}
		return 0
	}))

	// rune.load(path): Load a Lua script
	e.L.SetField(e.runeTable, "load", e.L.NewFunction(func(L *lua.LState) int {
		scriptPath := L.CheckString(1)
		e.events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionLoad, ScriptPath: scriptPath},
		}
		return 0
	}))
}

// registerTimerFuncs registers rune.timer.* functions
func (e *LuaEngine) registerTimerFuncs() {
	timerTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "timer", timerTable)

	// rune.timer.after(seconds, callback): Schedule delayed callback
	e.L.SetField(timerTable, "after", e.L.NewFunction(func(L *lua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2)

		time.AfterFunc(toDuration(seconds), func() {
			e.events <- mud.Event{
				Type: mud.EventTimer,
				Callback: func() {
					L.Push(fn)
					L.PCall(0, 0, nil)
				},
			}
		})
		return 0
	}))

	// rune.timer.every(seconds, callback): Schedule repeating callback, returns timer ID
	e.L.SetField(timerTable, "every", e.L.NewFunction(func(L *lua.LState) int {
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
					e.events <- mud.Event{
						Type: mud.EventTimer,
						Callback: func() {
							L.Push(fn)
							L.PCall(0, 0, nil)
						},
					}
				}
			}
		}()

		L.Push(lua.LNumber(id))
		return 1
	}))

	// rune.timer.cancel(id): Cancel a repeating timer
	e.L.SetField(timerTable, "cancel", e.L.NewFunction(func(L *lua.LState) int {
		id := int(L.CheckNumber(1))
		e.cancelTimer(id)
		return 0
	}))

	// rune.timer.cancel_all(): Cancel all repeating timers
	e.L.SetField(timerTable, "cancel_all", e.L.NewFunction(func(L *lua.LState) int {
		e.CancelAllTimers()
		return 0
	}))
}

// registerRegexFuncs registers rune.regex.* functions
func (e *LuaEngine) registerRegexFuncs() {
	regexTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "regex", regexTable)

	// rune.regex.match(pattern, text): Match using Go's regexp with LRU caching
	e.L.SetField(regexTable, "match", e.L.NewFunction(func(L *lua.LState) int {
		pattern := L.CheckString(1)
		text := L.CheckString(2)

		// Check LRU cache first
		re, ok := e.regexCache.Get(pattern)
		if !ok {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			e.regexCache.Add(pattern, re)
		}

		// FindStringSubmatch returns [full_match, group1, group2...]
		matches := re.FindStringSubmatch(text)
		if matches == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Convert []string to Lua Table
		tbl := L.NewTable()
		for i, m := range matches {
			// Lua arrays are 1-indexed
			tbl.RawSetInt(i+1, lua.LString(m))
		}

		L.Push(tbl)
		return 1
	}))
}

// getHooksCall returns the rune.hooks.call function
func (e *LuaEngine) getHooksCall() lua.LValue {
	hooksTable := e.L.GetField(e.runeTable, "hooks").(*lua.LTable)
	return e.L.GetField(hooksTable, "call")
}

// cancelTimer cancels a single timer by ID
func (e *LuaEngine) cancelTimer(id int) {
	e.timerMu.Lock()
	if entry, ok := e.timers[id]; ok {
		close(entry.done)
		entry.ticker.Stop()
		delete(e.timers, id)
	}
	e.timerMu.Unlock()
}

// toDuration converts Lua number seconds to Go duration
func toDuration(seconds lua.LNumber) time.Duration {
	return time.Duration(float64(seconds) * float64(time.Second))
}

// expandTilde expands ~ to home directory
func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
