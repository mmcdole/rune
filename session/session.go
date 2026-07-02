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
	"time"

	"github.com/mmcdole/rune/event"
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
	Output() <-chan network.Output
	LocalEchoEnabled() bool
}

var _ Network = (*network.TCPClient)(nil)

// Config holds session configuration
type Config struct {
	CoreScripts embed.FS // Embedded core Lua scripts
	ConfigDir   string   // Path to ~/.config/rune
	UserScripts []string // CLI script arguments
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
	historyLines []string
	historyLimit int

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
	events      chan event.Event
	timerEvents chan timer.Event
	barTicker   *time.Ticker

	// State
	lastPrompt    string
	config        Config
	clientState   lua.ClientState
	currentInput  string // Tracked so Lua can query via rune.input.get()
	currentCursor int    // Tracked so Lua can query via rune.input.get_cursor()
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net Network, uiInstance ui.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:          net,
		ui:           uiInstance,
		timer:        timer.NewService(timerEvents),
		timerEvents:  timerEvents,
		events:       make(chan event.Event, 256),
		config:       cfg,
		historyLines: make([]string, 0, 10000),
		historyLimit: 10000,
		sessionStore: make(map[string]string),
	}

	s.engine = lua.NewEngine(s)
	s.clientState.ScrollMode = "live"
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
		case ev := <-s.events:
			s.handleEvent(ev)
		case netOut := <-s.net.Output():
			s.handleNetworkOutput(netOut)
		case line := <-s.ui.Input():
			s.handleEvent(event.Event{Type: event.UserInput, Payload: event.Line(line)})
		case evt := <-s.timerEvents:
			s.engine.OnTimer(evt.ID)
		case <-s.barTicker.C:
			s.pushBarUpdates()
		case msg := <-s.ui.Outbound():
			s.handleUIMessage(msg)
		}
	}
}

// handleNetworkOutput converts network layer output to session events.
func (s *Session) handleNetworkOutput(out network.Output) {
	switch out.Kind {
	case network.OutputLine:
		s.handleEvent(event.Event{Type: event.NetLine, Payload: event.Line(out.Payload)})
	case network.OutputPrompt:
		s.handleEvent(event.Event{Type: event.NetPrompt, Payload: event.Line(out.Payload)})
	case network.OutputDisconnect:
		s.handleEvent(event.Event{Type: event.SysDisconnect})
	}
}

// handleEvent executes a single event on the session loop.
func (s *Session) handleEvent(ev event.Event) {
	switch ev.Type {
	case event.NetLine:
		payload := string(ev.Payload.(event.Line))
		line := text.NewLine(payload)
		if modified, show := s.engine.OnOutput(line); show {
			s.ui.Print(modified)
		}
		// Server line ends the prompt overlay
		s.lastPrompt = ""
		s.ui.SetPrompt("")

	case event.NetPrompt:
		// Commit previous prompt to scrollback before showing new one
		if s.lastPrompt != "" {
			s.ui.Print(s.lastPrompt)
		}
		payload := string(ev.Payload.(event.Line))
		line := text.NewLine(payload)
		modified := s.engine.OnPrompt(line)
		s.lastPrompt = modified
		s.ui.SetPrompt(modified)

	case event.UserInput:
		payload := string(ev.Payload.(event.Line))
		// Commit prompt to scrollback before processing input
		if s.lastPrompt != "" {
			s.ui.Print(s.lastPrompt)
			s.lastPrompt = ""
			s.ui.SetPrompt("")
		}
		if payload != "" {
			s.AddToHistory(payload)
		}
		if s.net.LocalEchoEnabled() {
			// Styling (and the choice to show the echo at all) is
			// Lua policy, dispatched through the "echo" hook.
			if styled, show := s.engine.OnEcho(payload); show {
				s.ui.Echo(styled)
			}
		}
		s.engine.OnInput(payload)

	case event.AsyncResult:
		if cb, ok := ev.Payload.(event.Callback); ok && cb != nil {
			cb()
		}

	case event.SysDisconnect:
		s.Disconnect()
	}
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
	if err := s.loadUserScripts(); err != nil {
		return err
	}
	s.engine.CallHook("ready")
	s.pushBindsAndLayout()
	s.pushBarUpdates()
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

// loadUserScripts loads init.lua and CLI-specified scripts.
func (s *Session) loadUserScripts() error {
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	if _, err := os.Stat(initPath); err == nil {
		if err := s.engine.DoFile(initPath); err != nil {
			return fmt.Errorf("init.lua: %w", err)
		}
	}

	for _, path := range s.config.UserScripts {
		if err := s.engine.DoFile(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
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
		s.currentCursor = m.Cursor
		s.engine.CallHook("input_changed", m.Text)
	case ui.CursorMovedMsg:
		s.currentCursor = m.Cursor
		// No Lua hook - cursor-only changes don't need Lua processing
	}
}
