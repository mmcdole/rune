package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/version"
)

// DefaultCallTimeout bounds each entry into the Lua VM. A script that
// exceeds it (e.g. an accidental infinite loop) is interrupted with an
// error instead of hanging the event loop forever.
const DefaultCallTimeout = 5 * time.Second

// Engine drives Rune's scripting environment through the engine-neutral
// script seam. It is a pure mechanism: it knows how to run Lua code and
// expose APIs. It does NOT know about core scripts, config dirs, or
// boot sequences.
type Engine struct {
	vm   script.Engine
	host Host

	// Cleared on reload to prevent stale script references
	pickerCallbacks map[string]script.FuncRef
	pickerNextID    int

	// Layout config, marshaled from rune.ui.layout calls
	barLayout ui.LayoutConfig

	// Re-applied after every Init so reloads keep it visible.
	configDir string

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

// NewEngine creates an Engine with a Host interface. All module and
// type declarations happen here, once; Init (re)creates the VM and the
// backend re-installs them.
func NewEngine(host Host) *Engine {
	e := &Engine{
		host:            host,
		vm:              newScriptEngine(),
		pickerCallbacks: make(map[string]script.FuncRef),
		barLayout:       ui.DefaultLayoutConfig(),
		CallTimeout:     DefaultCallTimeout,
	}
	e.registerAPIs()
	return e
}

// EngineBackend reports which scripting engine this binary was built
// with: "gopher-lua" (default) or "luajit" (-tags luajit).
func (e *Engine) EngineBackend() string { return e.vm.Backend() }

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
	e.vm.SetContext(ctx)
	defer func() {
		e.vm.RemoveContext()
		e.guardCancel()
		e.guardCancel = nil
		e.inLua = false
	}()

	err := fn()
	// The active context may have been replaced by pauseWatchdog, so
	// consult the VM's current context rather than the original.
	if lctx := e.vm.Context(); err != nil && lctx != nil && lctx.Err() != nil {
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
	e.vm.RemoveContext()
	e.guardCancel()

	fn()

	ctx, cancel := context.WithTimeout(context.Background(), e.CallTimeout)
	e.guardCancel = cancel
	e.vm.SetContext(ctx)
}

// Init initializes (or re-initializes) the VM with fresh state. The
// backend re-installs every registered module and type; loading
// scripts is the caller's job.
func (e *Engine) Init() error {
	if err := e.vm.Init(); err != nil {
		return err
	}

	e.host.TimerCancelAll()

	e.pickerCallbacks = make(map[string]script.FuncRef)
	e.pickerNextID = 0

	e.barLayout = ui.DefaultLayoutConfig()
	e.hooksBrokenReported = false

	if e.configDir != "" {
		e.vm.SetModuleField("rune", "config_dir", e.configDir)
	}
	return nil
}

// Close cleans up the VM.
func (e *Engine) Close() {
	e.host.TimerCancelAll()
	e.vm.Close()
}

// OnTimer handles wake-up calls from Session by dispatching to the
// Lua timer module, which owns the id -> callback mapping. Ids from a
// previous VM generation (or cancelled mid-flight) are ignored there.
func (e *Engine) OnTimer(id int) {
	if err := e.guard(func() error {
		_, _, err := e.vm.CallModule("rune.timer", "_fire", 0, id)
		return err
	}); err != nil {
		e.reportError("timer callback", err)
	}
}

// RegisterPickerCallback stores a pinned script function for later
// execution when the picker selection is made. Returns a callback ID.
func (e *Engine) RegisterPickerCallback(fn script.FuncRef) string {
	e.pickerNextID++
	id := fmt.Sprintf("p%d", e.pickerNextID)
	e.pickerCallbacks[id] = fn
	return id
}

// ExecutePickerCallback runs the script callback for a picker selection.
// Safe to call after reload - stale callbacks are silently ignored.
func (e *Engine) ExecutePickerCallback(id string, value string) {
	fn, ok := e.pickerCallbacks[id]
	if !ok {
		return
	}
	delete(e.pickerCallbacks, id)
	defer fn.Release()
	if err := e.guard(func() error {
		_, err := e.vm.Call(fn, 0, value)
		return err
	}); err != nil {
		e.reportError("picker callback", err)
	}
}

// CancelPickerCallback removes a picker callback without executing it.
func (e *Engine) CancelPickerCallback(id string) {
	if fn, ok := e.pickerCallbacks[id]; ok {
		fn.Release()
		delete(e.pickerCallbacks, id)
	}
}

// SetConfigDir exposes the config directory to scripts as
// rune.config_dir. Set as data, not generated source, so arbitrary
// path characters cannot break or inject code.
func (e *Engine) SetConfigDir(dir string) {
	e.configDir = dir
	e.vm.SetModuleField("rune", "config_dir", dir)
}

// DoString executes a raw string of Lua code.
// The name parameter is used for stack traces.
func (e *Engine) DoString(name, code string) error {
	return e.guard(func() error { return e.vm.DoString(name, code) })
}

// DoFile executes a Lua file from the filesystem. The backend extends
// the script search path so the file's local requires resolve.
func (e *Engine) DoFile(path string) error {
	return e.guard(func() error { return e.vm.DoFile(path) })
}

// OnInput handles traditional command input. It remains as a convenience for
// callers that do not need to construct an explicit submission.
func (e *Engine) OnInput(text string) {
	e.OnSubmission(input.Command(text))
}

// callHooks dispatches through rune.hooks.call; found=false means the
// hook system is unavailable (core failed to load or was clobbered).
func (e *Engine) callHooks(nret int, args ...any) ([]script.Result, bool, error) {
	var results []script.Result
	var found bool
	err := e.guard(func() error {
		var callErr error
		results, found, callErr = e.vm.CallModule("rune.hooks", "call", nret, args...)
		return callErr
	})
	return results, found, err
}

// OnSubmission dispatches one immutable input snapshot through Lua. Every
// input hook receives the same context shape; mode is always either "command"
// or "verbatim". Verbatim submissions still traverse user input hooks, but
// the core sender bypasses slash commands, aliases, repeats, and delimiters.
func (e *Engine) OnSubmission(submission input.Submission) {
	ctx := script.Tree{V: map[string]any{"mode": submission.Mode.String()}}

	// The consumed/pass-through result is dispatch routing state that
	// lives in Lua; nothing on the Go side acts on it.
	_, found, err := e.callHooks(1, "input", submission.Text, ctx)
	if !found {
		e.reportHooksBroken()
		if submission.Mode == input.ModeVerbatim {
			e.sendVerbatimFallback(submission.Text)
			return
		}

		// Degraded command mode keeps the escape hatches working and passes
		// everything else to the server as a plain telnet client.
		switch submission.Text {
		case "/quit":
			e.host.Quit()
		case "/reload":
			e.host.Reload()
		default:
			_ = e.host.Send(submission.Text)
		}
		return
	}
	if err != nil {
		e.reportError("input dispatch", err)
	}
}

// sendVerbatimFallback is the no-Lua escape hatch. strings.Split preserves
// leading, adjacent, and trailing empty lines and treats only LF as a boundary.
func (e *Engine) sendVerbatimFallback(input string) {
	for _, line := range strings.Split(input, "\n") {
		_ = e.host.Send(line)
	}
}

// OnEcho styles the local echo of typed input by dispatching the
// "echo" hook: presentation belongs to Lua, so the "> " prefix and
// color live in the core echo handler, and user handlers may rewrite
// or hide the echo. Degraded mode falls back to Go-side styling so
// input stays visible.
func (e *Engine) OnEcho(in string) (string, bool) {
	// Echo is a presentation boundary. Preserve canonical submission bytes
	// elsewhere, but never let pasted terminal controls reach either Lua
	// styling or the degraded Go fallback as executable sequences.
	in = text.VisualizeTerminalControls(in, true)
	fallback := text.Green("> " + in)

	results, found, err := e.callHooks(2, "echo", in)
	if !found {
		e.reportHooksBroken()
		return fallback, true
	}
	if err != nil {
		e.reportError("echo dispatch", err)
		return fallback, true
	}

	modified, show := results[0], results[1]
	if show.False() {
		return "", false
	}
	return modified.String(), true
}

// OnOutput handles server text.
func (e *Engine) OnOutput(line text.Line) (string, bool) {
	results, found, err := e.callHooks(2, "output", script.Obj{Type: "line", Payload: &line})
	if !found {
		e.reportHooksBroken()
		return line.Raw, true
	}
	if err != nil {
		e.reportError("output dispatch", err)
		return line.Raw, true
	}

	modified, show := results[0], results[1]
	if show.False() {
		return "", false
	}
	return modified.String(), true
}

// OnPrompt handles server prompts.
func (e *Engine) OnPrompt(line text.Line) string {
	results, found, err := e.callHooks(2, "prompt", script.Obj{Type: "line", Payload: &line})
	if !found {
		e.reportHooksBroken()
		return line.Raw
	}
	if err != nil {
		e.reportError("prompt dispatch", err)
		return line.Raw
	}

	modified, show := results[0], results[1]
	if show.False() {
		return ""
	}
	return modified.String()
}

// OnGMCP dispatches a GMCP message to Lua: the raw JSON is decoded
// into a Go tree so handlers receive a real Lua value, plus the
// original raw text for anyone who wants it. Malformed JSON is
// reported once and the message dropped - a broken server message
// must not take anything else down.
func (e *Engine) OnGMCP(pkg, raw string) {
	var value any
	if raw != "" {
		var decoded any
		if err := json.Unmarshal([]byte(escapeRawJSONControlsInStrings(raw)), &decoded); err != nil {
			e.reportError("gmcp "+pkg, fmt.Errorf("malformed JSON: %w", err))
			return
		}
		value = decoded
	}

	if err := e.guard(func() error {
		_, _, err := e.vm.CallModule("rune.gmcp", "_dispatch", 0, pkg, script.Tree{V: value}, raw)
		return err
	}); err != nil {
		e.reportError("gmcp dispatch", err)
	}
}

// escapeRawJSONControlsInStrings tolerates servers which put terminal control
// bytes directly in JSON strings instead of encoding them. Aardwolf does this
// with ANSI ESC bytes in colored Comm.Channel messages. Escaping them for the
// decoder preserves the bytes in the decoded value; OnGMCP still passes the
// original text to handlers as raw.
//
// Controls outside strings and controls following a JSON escape introducer are
// left alone so otherwise malformed JSON remains malformed.
func escapeRawJSONControlsInStrings(raw string) string {
	const hex = "0123456789abcdef"

	inString := false
	escaped := false
	last := 0
	var repaired strings.Builder

	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			continue
		}

		if escaped {
			escaped = false
			continue
		}
		switch {
		case c == '\\':
			escaped = true
		case c == '"':
			inString = false
		case c < 0x20:
			repaired.WriteString(raw[last:i])
			repaired.WriteString(`\u00`)
			repaired.WriteByte(hex[c>>4])
			repaired.WriteByte(hex[c&0x0f])
			last = i + 1
		}
	}

	if last == 0 {
		return raw
	}
	repaired.WriteString(raw[last:])
	return repaired.String()
}

// CallHook calls a hook event with string arguments.
func (e *Engine) CallHook(event string, args ...string) {
	callArgs := make([]any, len(args)+1)
	callArgs[0] = event
	for i, arg := range args {
		callArgs[i+1] = arg
	}

	_, found, err := e.callHooks(0, callArgs...)
	if !found {
		e.reportHooksBroken()
		// Errors must never disappear, even with hooks broken.
		if event == "error" {
			e.host.Print(text.Red("[Error] " + strings.Join(args, " ")))
		}
		return
	}
	if err != nil {
		e.reportError("'"+event+"' hook", err)
	}
}

func (e *Engine) registerAPIs() {
	// Data fields (not API): the client version, single-sourced from
	// the version package so TTYPE/MNES and /version cannot drift, and
	// the scripting engine this binary was built with.
	e.vm.RegisterModule("rune", nil, map[string]any{
		"version": version.Number,
		"engine":  e.vm.Backend(),
	})

	registerLineType(e.vm)
	e.registerCoreFuncs()
	e.registerTimerFuncs()
	e.registerRegexFuncs()
	e.registerUIFuncs()
	e.registerStateFuncs()
	e.registerBarFuncs()
	e.registerPickerFuncs()
	e.registerHistoryFuncs()
	e.registerInputFuncs()
	e.registerSessionFuncs()
	e.registerStoreFuncs()
	e.registerLogFuncs()
	e.registerGMCPFuncs()
	e.registerHTTPFuncs()
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
