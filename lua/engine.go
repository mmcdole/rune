package lua

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	glua "github.com/yuin/gopher-lua"
)

//go:embed core/*.lua
var CoreScripts embed.FS

// --- Types ---

// timerEntry holds a timer and its done channel for clean shutdown
type timerEntry struct {
	ticker *time.Ticker
	done   chan struct{}
}

// Engine implements the ScriptEngine interface
type Engine struct {
	L          *glua.LState
	regexCache *lru.Cache[string, *regexp.Regexp]
	timers     map[int]*timerEntry
	timerID    int
	timerMu    sync.Mutex

	// Cached table reference
	runeTable *glua.LTable

	// Host interface for communication with the rest of the system
	host Host
}

// --- Constructor ---

// NewEngine initializes a Lua VM with regex caching and timer management.
func NewEngine(host Host) *Engine {
	cache, _ := lru.New[string, *regexp.Regexp](100)
	return &Engine{
		regexCache: cache,
		timers:     make(map[int]*timerEntry),
		host:       host,
	}
}

// --- Lifecycle ---

// InitState initializes (or re-initializes) the Lua VM with fresh state
func (e *Engine) InitState(coreScripts embed.FS, configDir string) error {
	// Cancel all timers
	e.CancelAllTimers()

	// Close old Lua state if it exists
	if e.L != nil {
		e.L.Close()
	}

	// Create fresh Lua state
	e.L = glua.NewState()
	cache, _ := lru.New[string, *regexp.Regexp](100)
	e.regexCache = cache
	e.timers = make(map[int]*timerEntry)
	e.timerID = 0

	// Register custom types
	registerLineType(e.L)

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
func (e *Engine) Close() {
	e.CancelAllTimers()
	e.L.Close()
}

// --- Script Loading ---

// SetConfigDir sets the rune.config_dir variable for user scripts
func (e *Engine) SetConfigDir(dir string) {
	e.L.SetField(e.runeTable, "config_dir", glua.LString(dir))
}

// LoadEmbeddedCore loads core scripts from embedded filesystem
func (e *Engine) LoadEmbeddedCore(scripts embed.FS) error {
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
func (e *Engine) LoadUserScripts(paths []string) error {
	for _, path := range paths {
		path = expandTilde(path)

		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", path, err)
		}
		dir := filepath.Dir(absPath)

		// Temporarily prepend script's directory to package.path
		pkg := e.L.GetGlobal("package").(*glua.LTable)
		oldPath := e.L.GetField(pkg, "path").String()
		newPath := dir + "/?.lua;" + oldPath
		e.L.SetField(pkg, "path", glua.LString(newPath))

		content, err := os.ReadFile(absPath)
		if err != nil {
			e.L.SetField(pkg, "path", glua.LString(oldPath))
			return fmt.Errorf("reading %s: %w", absPath, err)
		}

		if err := e.L.DoString(string(content)); err != nil {
			e.L.SetField(pkg, "path", glua.LString(oldPath))
			return fmt.Errorf("executing %s: %w", absPath, err)
		}

		e.L.SetField(pkg, "path", glua.LString(oldPath))
	}
	return nil
}

// --- Event Handlers ---

// OnInput handles user typing
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

// OnOutput handles server text
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

// OnPrompt handles server prompts
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

// CallHook calls a hook event with string arguments
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

// ExecuteCallback runs a stored Lua function
func (e *Engine) ExecuteCallback(cb func()) {
	if cb != nil {
		cb()
	}
}

// --- Timer Management ---

// CancelAllTimers stops all repeating timers
func (e *Engine) CancelAllTimers() {
	e.timerMu.Lock()
	for id, entry := range e.timers {
		close(entry.done)
		entry.ticker.Stop()
		delete(e.timers, id)
	}
	e.timerMu.Unlock()
}

// --- Private Helpers ---

// registerHostFuncs binds Lua API functions to the rune table
func (e *Engine) registerHostFuncs() {
	e.runeTable = e.L.NewTable()
	e.L.SetGlobal("rune", e.runeTable)

	e.registerCoreFuncs()
	e.registerTimerFuncs()
	e.registerRegexFuncs()
	e.registerStatusFuncs()
	e.registerPaneFuncs()
	e.registerInfobarFuncs()
}

// getHooksCall returns the rune.hooks.call function
func (e *Engine) getHooksCall() glua.LValue {
	hooksTable := e.L.GetField(e.runeTable, "hooks").(*glua.LTable)
	return e.L.GetField(hooksTable, "call")
}

// clearRequireCache clears the Lua require cache so modules reload fresh
func (e *Engine) clearRequireCache() {
	pkg := e.L.GetGlobal("package").(*glua.LTable)
	loaded := e.L.GetField(pkg, "loaded").(*glua.LTable)
	loaded.ForEach(func(key, value glua.LValue) {
		e.L.SetField(loaded, key.String(), glua.LNil)
	})
}

// stripAnsi removes ANSI escape codes from a string
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

// expandTilde expands ~ to home directory
func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
