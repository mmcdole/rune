package engine

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/drake/rune/mud"
	lua "github.com/yuin/gopher-lua"
)

// LuaEngine implements the ScriptEngine interface
type LuaEngine struct {
	L          *lua.LState
	regexCache map[string]*regexp.Regexp
	timers     map[int]*time.Ticker
	timerID    int
	timerMu    sync.Mutex
}

// NewLuaEngine initializes a Lua VM with regex caching and timer management.
func NewLuaEngine() *LuaEngine {
	return &LuaEngine{
		L:          lua.NewState(),
		regexCache: make(map[string]*regexp.Regexp),
		timers:     make(map[int]*time.Ticker),
		timerID:    0,
	}
}

// SetConfigDir sets the rune.config_dir variable for user scripts
func (e *LuaEngine) SetConfigDir(dir string) {
	runeTable := e.L.GetGlobal("rune").(*lua.LTable)
	e.L.SetField(runeTable, "config_dir", lua.LString(dir))
}

// RegisterHostFuncs binds Go functions to the rune namespace
func (e *LuaEngine) RegisterHostFuncs(events chan<- mud.Event, uplink chan<- string, display chan<- string) {
	runeTable := e.L.NewTable()
	e.L.SetGlobal("rune", runeTable)

	// rune.send_raw(text): Bypasses alias processing, writes directly to socket
	e.L.SetField(runeTable, "send_raw", e.L.NewFunction(func(L *lua.LState) int {
		cmd := L.CheckString(1)
		uplink <- cmd
		return 0
	}))

	// rune.print(text): Outputs text to the local display
	e.L.SetField(runeTable, "print", e.L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		display <- msg
		return 0
	}))

	// rune.quit(): Exit the client
	e.L.SetField(runeTable, "quit", e.L.NewFunction(func(L *lua.LState) int {
		events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionQuit},
		}
		return 0
	}))

	// rune.connect(address): Connect to server
	e.L.SetField(runeTable, "connect", e.L.NewFunction(func(L *lua.LState) int {
		addr := L.CheckString(1)
		events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionConnect, Address: addr},
		}
		return 0
	}))

	// rune.disconnect(): Disconnect from server
	e.L.SetField(runeTable, "disconnect", e.L.NewFunction(func(L *lua.LState) int {
		events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionDisconnect},
		}
		return 0
	}))

	// rune.reload(): Reload all scripts
	e.L.SetField(runeTable, "reload", e.L.NewFunction(func(L *lua.LState) int {
		events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionReload},
		}
		return 0
	}))

	// rune.load(path): Load a Lua script with proper path setup for relative requires
	e.L.SetField(runeTable, "load", e.L.NewFunction(func(L *lua.LState) int {
		scriptPath := L.CheckString(1)
		events <- mud.Event{
			Type:    mud.EventSystemControl,
			Control: mud.ControlOp{Action: mud.ActionLoad, ScriptPath: scriptPath},
		}
		return 0
	}))

	// Create rune.timer submodule
	timerTable := e.L.NewTable()
	e.L.SetField(runeTable, "timer", timerTable)

	// rune.timer.after(seconds, callback): Schedule delayed callback
	e.L.SetField(timerTable, "after", e.L.NewFunction(func(L *lua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2)

		time.AfterFunc(time.Duration(float64(seconds)*float64(time.Second)), func() {
			events <- mud.Event{
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
		ticker := time.NewTicker(time.Duration(float64(seconds) * float64(time.Second)))
		e.timers[id] = ticker
		e.timerMu.Unlock()

		go func() {
			for range ticker.C {
				events <- mud.Event{
					Type: mud.EventTimer,
					Callback: func() {
						L.Push(fn)
						L.PCall(0, 0, nil)
					},
				}
			}
		}()

		L.Push(lua.LNumber(id))
		return 1
	}))

	// rune.timer.cancel(id): Cancel a repeating timer
	e.L.SetField(timerTable, "cancel", e.L.NewFunction(func(L *lua.LState) int {
		id := int(L.CheckNumber(1))

		e.timerMu.Lock()
		if ticker, ok := e.timers[id]; ok {
			ticker.Stop()
			delete(e.timers, id)
		}
		e.timerMu.Unlock()

		return 0
	}))

	// rune.timer.cancel_all(): Cancel all repeating timers
	e.L.SetField(timerTable, "cancel_all", e.L.NewFunction(func(L *lua.LState) int {
		e.timerMu.Lock()
		for id, ticker := range e.timers {
			ticker.Stop()
			delete(e.timers, id)
		}
		e.timerMu.Unlock()

		return 0
	}))

	// Create rune.regex submodule
	regexTable := e.L.NewTable()
	e.L.SetField(runeTable, "regex", regexTable)

	// rune.regex.match(pattern, text): Match using Go's regexp with caching
	e.L.SetField(regexTable, "match", e.L.NewFunction(func(L *lua.LState) int {
		pattern := L.CheckString(1)
		text := L.CheckString(2)

		// Check cache first
		re, ok := e.regexCache[pattern]
		if !ok {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			e.regexCache[pattern] = re
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

// LoadEmbeddedCore loads core scripts from embedded filesystem
func (e *LuaEngine) LoadEmbeddedCore(scripts embed.FS) error {
	// Read all files from core directory
	entries, err := fs.ReadDir(scripts, "core")
	if err != nil {
		return err
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
			return err
		}
		if err := e.L.DoString(string(content)); err != nil {
			return err
		}
	}

	return nil
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

// LoadUserScripts loads user scripts from filesystem paths
func (e *LuaEngine) LoadUserScripts(paths []string) error {
	for _, path := range paths {
		// Expand ~ to home directory
		path = expandTilde(path)

		// Get absolute path and directory
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
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
			return err
		}

		if err := e.L.DoString(string(content)); err != nil {
			e.L.SetField(pkg, "path", lua.LString(oldPath))
			return err
		}

		// Restore original package.path
		e.L.SetField(pkg, "path", lua.LString(oldPath))
	}
	return nil
}

// OnInput handles user typing
func (e *LuaEngine) OnInput(text string) bool {
	if err := e.L.CallByParam(lua.P{
		Fn:      e.L.GetGlobal("on_input"),
		NRet:    0,
		Protect: true,
	}, lua.LString(text)); err != nil {
		return false
	}
	return true
}

// OnOutput handles server text
func (e *LuaEngine) OnOutput(text string) (string, bool) {
	if err := e.L.CallByParam(lua.P{
		Fn:      e.L.GetGlobal("on_output"),
		NRet:    1,
		Protect: true,
	}, lua.LString(text)); err != nil {
		return text, true
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	if ret == lua.LNil {
		return "", false
	}

	return ret.String(), true
}

// OnPrompt handles server prompts
func (e *LuaEngine) OnPrompt(text string) string {
	fn := e.L.GetGlobal("on_prompt")
	if fn == lua.LNil {
		return text
	}

	if err := e.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, lua.LString(text)); err != nil {
		return text
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	if ret == lua.LNil {
		return text
	}

	return ret.String()
}

// CallHook calls a Lua hook function by name with string arguments
func (e *LuaEngine) CallHook(name string, args ...string) {
	fn := e.L.GetGlobal(name)
	if fn == lua.LNil {
		return
	}

	luaArgs := make([]lua.LValue, len(args))
	for i, arg := range args {
		luaArgs[i] = lua.LString(arg)
	}

	e.L.CallByParam(lua.P{
		Fn:      fn,
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

// CancelAllTimers stops all repeating timers
func (e *LuaEngine) CancelAllTimers() {
	e.timerMu.Lock()
	for id, ticker := range e.timers {
		ticker.Stop()
		delete(e.timers, id)
	}
	e.timerMu.Unlock()
}

// ClearRequireCache clears the Lua require cache so modules reload fresh
func (e *LuaEngine) ClearRequireCache() {
	pkg := e.L.GetGlobal("package").(*lua.LTable)
	loaded := e.L.GetField(pkg, "loaded").(*lua.LTable)
	// Clear all loaded modules
	loaded.ForEach(func(key, value lua.LValue) {
		e.L.SetField(loaded, key.String(), lua.LNil)
	})
}

// Reload reloads all scripts (core + user init)
func (e *LuaEngine) Reload(coreScripts embed.FS, configDir string) error {
	// Cancel all timers
	e.CancelAllTimers()
	// Clear require cache
	e.ClearRequireCache()
	// Reload core scripts
	if err := e.LoadEmbeddedCore(coreScripts); err != nil {
		return err
	}
	// Reload init.lua
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
