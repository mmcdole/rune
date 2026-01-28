package session

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/drake/rune/event"
	"github.com/drake/rune/lua"
	"github.com/drake/rune/network"
	"github.com/drake/rune/text"
	"github.com/drake/rune/timer"
	"github.com/drake/rune/ui"
)

// Compile-time interface check - Session implements lua.Host
var _ lua.Host = (*Session)(nil)

// UI is imported from the ui package.
// See ui/interface.go for the full interface definition.


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
	net   *network.TCPClient
	ui    ui.UI
	timer *timer.Service

	// Scripting
	engine *lua.Engine

	// History
	historyLines []string
	historyLimit int

	// Channels
	events      chan event.Event
	timerEvents chan timer.Event
	barTicker   *time.Ticker

	// State
	lastPrompt   string
	config       Config
	cancel       context.CancelFunc
	clientState  lua.ClientState
	currentInput  string // Tracked so Lua can query via rune.input.get()
	currentCursor int    // Tracked so Lua can query via rune.input.get_cursor()
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net *network.TCPClient, uiInstance ui.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:         net,
		ui:          uiInstance,
		timer:       timer.NewService(timerEvents),
		timerEvents: timerEvents,
		events:      make(chan event.Event, 256),
		config:       cfg,
		historyLines: make([]string, 0, 10000),
		historyLimit: 10000,
	}

	s.engine = lua.NewEngine(s)
	s.clientState.ScrollMode = "live"

	return s
}

// Run starts the session and blocks until exit.
func (s *Session) Run(ctx context.Context) error {
	// Derive cancellable context for internal shutdown (rune.quit)
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	defer func() {
		cancel()
		s.engine.Close()
		if s.barTicker != nil {
			s.barTicker.Stop()
		}
		s.timer.CancelAll()
		s.net.Disconnect()
		s.ui.Quit()
	}()

	if err := s.boot(); err != nil {
		s.ui.Print(fmt.Sprintf("\033[31m[System] Boot Error: %v\033[0m", err))
	}

	s.barTicker = time.NewTicker(250 * time.Millisecond)

	go s.processEvents(ctx)

	return s.ui.Run()
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
			s.engine.OnTimer(evt.ID, evt.Repeating)
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
			s.ui.Echo(payload)
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
	setupCode := fmt.Sprintf("rune.config_dir = [[%s]]", s.config.ConfigDir)
	return s.engine.DoString("boot_config", setupCode)
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

// renderBars calls all Lua bar renderers and returns their content.
// Must be called from Session goroutine (thread-safe Lua access).
func (s *Session) renderBars(width int) map[string]ui.BarContent {
	names := s.engine.GetBarNames()
	if len(names) == 0 {
		return nil
	}

	result := make(map[string]ui.BarContent, len(names))
	for _, name := range names {
		if content, ok := s.engine.RenderBar(name, width); ok {
			result[name] = content
		}
	}
	return result
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

	content := s.renderBars(width)
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
