package session

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/timer"
	"github.com/mmcdole/rune/ui"
)

// Compile-time interface check - Session implements lua.Host
var _ lua.Host = (*Session)(nil)

// Network is the connection layer Session drives. In production this
// is *network.TCPClient; tests substitute a mock so the event loop
// can be exercised without sockets.
type Network interface {
	Connect(ctx context.Context, address string) error
	Disconnect()
	Send(data string) error
	SendGMCP(pkg, data string) error
	GMCPActive() bool
	SetWindowSize(width, height int)
	Output() <-chan network.Output
	LocalEchoEnabled() bool
}

var _ Network = (*network.TCPClient)(nil)

// Config holds session configuration
type Config struct {
	CoreScripts   embed.FS // Embedded core Lua scripts
	ConfigDir     string   // Directory for all of Rune's files (init.lua, store.json, worlds, logs)
	ConnectTarget string   // CLI connect target (world, host port, or address)
}

// Session is the central actor/orchestrator that owns the Lua state and
// processes all events sequentially via a single goroutine. It implements
// lua.Host to provide Lua scripts access to network, UI, timers, and system
// operations.
type Session struct {
	// Infrastructure
	net   Network
	ui    ui.UI
	timer *timer.Service

	// Scripting
	engine *lua.Engine

	// History
	historyEntries []input.Submission
	historyLimit   int

	// Reload-surviving Lua state (see lua_session.go)
	sessionStore map[string]string

	// Durable store backed by <config>/store.json (see lua_store.go)
	store        map[string]json.RawMessage
	storePath    string
	storeLoadErr error // corrupt/unreadable store.json, reported at boot

	// Active session log (see lua_log.go); survives /reload
	logFile *os.File
	logPath string

	// Channels
	// asyncResults marshals work from producer goroutines (dial, HTTP,
	// deferred reload) back onto the session goroutine, which runs each
	// callback with exclusive access to the Lua state.
	asyncResults chan func()
	timerEvents  chan timer.Event
	barTicker    *time.Ticker

	// State
	lastPrompt    string
	connectTarget string // CLI connect target; consumed on first boot only
	config        Config
	clientState   lua.ClientState
	currentInput  string // Tracked so Lua can query via rune.input.get()
	currentCursor int    // Zero-based UTF-8 byte offset exposed to Lua
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net Network, uiInstance ui.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:            net,
		ui:             uiInstance,
		timer:          timer.NewService(timerEvents),
		timerEvents:    timerEvents,
		asyncResults:   make(chan func(), 256),
		config:         cfg,
		historyEntries: make([]input.Submission, 0, 10000),
		historyLimit:   10000,
		sessionStore:   make(map[string]string),
	}

	s.engine = lua.NewEngine(s)
	s.clientState.ScrollMode = "live"
	s.connectTarget = cfg.ConnectTarget
	s.loadStore()

	return s
}

// Run starts the session and blocks until exit.
func (s *Session) Run(ctx context.Context) error {
	// Derive a cancellable context so Run can stop the event loop
	// before tearing down the engine.
	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
		s.engine.Close()
		if s.barTicker != nil {
			s.barTicker.Stop()
		}
		s.timer.Stop()
		s.net.Disconnect()
		s.LogStop()
		s.ui.Quit()
	}()

	if err := s.boot(); err != nil {
		s.ui.Print(text.Red(fmt.Sprintf("[System] Boot Error: %v", err)))
	}

	s.barTicker = time.NewTicker(250 * time.Millisecond)

	eventLoopDone := make(chan struct{})
	go func() {
		defer close(eventLoopDone)
		s.processEvents(ctx)
	}()

	err := s.ui.Run()

	// Join the event loop before the deferred engine.Close tears down
	// the Lua state: processEvents may be mid-call into the VM. UI
	// sends are no-ops after Run returns, and Lua execution is bounded
	// by the engine watchdog, so this cannot block indefinitely.
	cancel()
	<-eventLoopDone

	return err
}

// processEvents is the main event loop.
func (s *Session) processEvents(ctx context.Context) {
	for {
		// Priority: drain UI input messages first (for responsive completion)
		select {
		case msg := <-s.ui.Outbound():
			s.handleUIMessage(msg)
			continue
		default:
		}

		select {
		case <-ctx.Done():
			return
		case cb := <-s.asyncResults:
			cb()
		case netOut := <-s.net.Output():
			s.handleNetworkOutput(netOut)
		case submission := <-s.ui.Input():
			s.handleSubmission(submission)
		case evt := <-s.timerEvents:
			s.engine.OnTimer(evt.ID)
		case <-s.barTicker.C:
			s.pushBarUpdates()
		case msg := <-s.ui.Outbound():
			s.handleUIMessage(msg)
		}
	}
}

// handleNetworkOutput dispatches network layer output on the session loop.
func (s *Session) handleNetworkOutput(out network.Output) {
	switch out.Kind {
	case network.OutputLine:
		s.handleServerLine(out.Payload)
	case network.OutputPrompt:
		s.handleServerPrompt(out.Payload)
	case network.OutputDisconnect:
		s.Disconnect()
	case network.OutputGMCP:
		s.engine.OnGMCP(out.Package, out.Payload)
	case network.OutputGMCPEnabled:
		s.engine.CallHook("gmcp_enabled")
	}
}

// handleServerLine processes a complete server line.
func (s *Session) handleServerLine(payload string) {
	line := text.NewLine(payload)
	if modified, show := s.engine.OnOutput(line); show {
		// Display egress owns terminal safety: strip everything but
		// SGR so server clear/cursor sequences cannot wipe UI chrome
		// (issue #69). Lua hooks above saw the raw line.
		s.ui.Print(text.SanitizeDisplay(modified))
	}
	// Server line ends the prompt overlay
	s.lastPrompt = ""
	s.ui.SetPrompt("")
}

// handleServerPrompt processes a prompt snapshot. It replaces the overlay
// and is never committed to scrollback here. In unterminated mode snapshots
// are cumulative peeks of the growing line, so committing a superseded one
// would turn socket read boundaries into visible lines (issue #25); a
// GA/EOR prompt superseding another is a repaint and gets the same
// treatment. Only input submission commits the active prompt
// (handleSubmission).
func (s *Session) handleServerPrompt(payload string) {
	line := text.NewLine(payload)
	// Sanitized before storing so the overlay and the later
	// scrollback commit (handleSubmission) both stay chrome-safe.
	modified := text.SanitizeDisplay(s.engine.OnPrompt(line))
	s.lastPrompt = modified
	s.ui.SetPrompt(modified)
}

// handleSubmission processes an immutable input snapshot. Command submissions
// retain Rune's normal aliases, delimiters, repeats, and slash commands;
// verbatim submissions bypass that interpretation and send physical lines as
// written.
func (s *Session) handleSubmission(submission input.Submission) {
	// Commit prompt to scrollback before processing input.
	if s.lastPrompt != "" {
		s.ui.Print(s.lastPrompt)
		s.lastPrompt = ""
		s.ui.SetPrompt("")
	}
	s.addHistorySubmission(submission)
	if s.net.LocalEchoEnabled() {
		lines := []string{submission.Text}
		if submission.Mode == input.ModeVerbatim {
			// Scrollback entries must be physical lines. An embedded LF in
			// one entry would render extra terminal rows without the viewport
			// accounting for them.
			lines = strings.Split(submission.Text, "\n")
		}
		for _, line := range lines {
			// Styling (and the choice to show the echo at all) is Lua
			// policy, dispatched through the "echo" hook. Engine.OnEcho owns
			// the safe display projection; canonical bytes remain untouched
			// here for history and the wire.
			if styled, show := s.engine.OnEcho(line); show {
				s.ui.Echo(styled)
			}
		}
	}

	s.engine.OnSubmission(submission)
}

// boot loads the VM state.
func (s *Session) boot() error {
	if s.storeLoadErr != nil {
		s.ui.Print(text.Red("[System] " + s.storeLoadErr.Error()))
		s.storeLoadErr = nil
	}
	if err := s.initLua(); err != nil {
		return err
	}
	if err := s.loadCoreScripts(); err != nil {
		return err
	}
	// User scripts must never abort boot: a syntax error in a new
	// user's first init.lua would otherwise skip the ready hook and
	// the binds/layout push, leaving a half-dead client. Each failure
	// is reported individually and the rest of boot proceeds.
	s.loadUserScript()
	s.engine.CallHook("ready")
	s.pushBindsAndLayout()
	s.pushBarUpdates()

	// CLI connect target: routed through the /connect command so a
	// world name, "host port", or address all resolve identically.
	// Consumed on the first boot only - /reload must not reconnect.
	if s.connectTarget != "" {
		target := s.connectTarget
		s.connectTarget = ""
		s.engine.OnInput("/connect " + target)
	}
	return nil
}

// initLua initializes the Lua VM and sets up config.
func (s *Session) initLua() error {
	if err := s.engine.Init(); err != nil {
		return err
	}
	s.engine.SetConfigDir(s.config.ConfigDir)
	return nil
}

// loadCoreScripts loads embedded core Lua scripts in sorted order.
func (s *Session) loadCoreScripts() error {
	entries, err := fs.ReadDir(s.config.CoreScripts, "core")
	if err != nil {
		return fmt.Errorf("reading core scripts: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, file := range files {
		content, err := s.config.CoreScripts.ReadFile("core/" + file)
		if err != nil {
			return fmt.Errorf("core/%s: %w", file, err)
		}
		if err := s.engine.DoString(file, string(content)); err != nil {
			return fmt.Errorf("core/%s: %w", file, err)
		}
	}
	return nil
}

// loadUserScript loads init.lua. A failure never propagates: the client must come
// up fully functional (binds, bars, ready hook) around a broken user
// script, so the user can fix it and /reload.
func (s *Session) loadUserScript() {
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	if _, err := os.Stat(initPath); err == nil {
		if err := s.engine.DoFile(initPath); err != nil {
			s.reportScriptError("init.lua", err)
		}
	}
}

// reportScriptError surfaces a user-script load failure. Printed
// directly (not via the "error" hook) so it is visible even when the
// failed script broke the hook system.
func (s *Session) reportScriptError(name string, err error) {
	s.ui.Print(text.Red(fmt.Sprintf("[Script Error] %s: %v", name, err)))
	s.ui.Print(text.Red("  the rest of the client loaded normally - fix the script and /reload"))
}

// handleKeyBind executes a Lua key binding.
// Must be called from Session goroutine (thread-safe Lua access).
func (s *Session) handleKeyBind(key string) {
	s.engine.HandleKeyBind(key)
}

// pushBarUpdates renders all Lua bars and pushes to UI.
func (s *Session) pushBarUpdates() {
	width := s.clientState.Width
	if width <= 0 {
		width = 80
	}

	content := s.engine.RenderBars(width)
	if content != nil {
		s.ui.UpdateBars(content)
	}
}

// pushBindsAndLayout pushes current bindings and layout config to UI.
func (s *Session) pushBindsAndLayout() {
	keys := s.engine.GetBoundKeys()
	bindsMap := make(map[string]bool, len(keys))
	for _, key := range keys {
		bindsMap[key] = true
	}
	s.ui.UpdateBinds(bindsMap)

	luaLayout := s.engine.GetLayout()
	if len(luaLayout.Top) > 0 || len(luaLayout.Bottom) > 0 {
		s.ui.UpdateLayout(luaLayout.Top, luaLayout.Bottom)
	}
}

// handleUIMessage processes messages from the UI.
func (s *Session) handleUIMessage(msg ui.UIEvent) {
	switch m := msg.(type) {
	case ui.ExecuteBindMsg:
		s.handleKeyBind(string(m))
	case ui.WindowSizeChangedMsg:
		s.clientState.Width = m.Width
		s.clientState.Height = m.Height
		s.net.SetWindowSize(m.Width, m.Height)
		s.engine.UpdateState(s.clientState)
		s.pushBarUpdates()
	case ui.ScrollStateChangedMsg:
		s.clientState.ScrollMode = m.Mode
		s.clientState.ScrollLines = m.NewLines
		s.engine.UpdateState(s.clientState)
		s.pushBarUpdates()
	case ui.PickerSelectMsg:
		s.handlePickerResult(m.CallbackID, m.Value, m.Accepted)
	case ui.InputChangedMsg:
		s.currentInput = m.Text
		s.currentCursor = input.RuneCursorToByte(m.Text, m.Cursor)
		s.engine.CallHook("input_changed", m.Text)
	case ui.CursorMovedMsg:
		s.currentCursor = input.RuneCursorToByte(s.currentInput, m.Cursor)
		// No Lua hook - cursor-only changes don't need Lua processing
	}
}
