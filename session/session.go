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

// Network defines the TCP/Telnet layer.
type Network interface {
	Connect(ctx context.Context, address string) error
	Disconnect()
	Send(data string) error
	Output() <-chan network.Output
}

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

// Session orchestrates the MUD client components.
type Session struct {
	// Infrastructure
	net   Network
	ui    ui.UI
	timer *timer.Service

	// Scripting
	engine *lua.Engine

	// Managers
	history         *HistoryManager
	pickerCallbacks map[string]func(string)
	pickerNextID    int

	// Channels
	events      chan event.Event
	timerEvents chan timer.Event
	barTicker   *time.Ticker // Periodic bar re-render ticker

	// Track last prompt overlay to commit to history when replaced
	lastPrompt string

	// Config (retained for reload)
	config Config

	// Shutdown coordination
	cancel context.CancelFunc

	// Client state (for Lua rune.state access)
	clientState lua.ClientState

	// Current input content (tracked for rune.input.get())
	currentInput string
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net Network, uiInstance ui.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:             net,
		ui:              uiInstance,
		timer:           timer.NewService(timerEvents),
		timerEvents:     timerEvents,
		events:          make(chan event.Event, 256),
		config:          cfg,
		history:         NewHistoryManager(10000),
		pickerCallbacks: make(map[string]func(string)),
	}


	// Create engine with Session as Host
	s.engine = lua.NewEngine(s)

	// Initialize client state defaults
	s.clientState.ScrollMode = "live"

	return s
}

// Run starts the session and blocks until exit.
func (s *Session) Run(ctx context.Context) error {
	// Derive a cancellable context for internal shutdown (rune.quit)
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Ensure cleanup runs when context ends
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

	// Boot the system
	if err := s.boot(); err != nil {
		s.ui.Print(fmt.Sprintf("\033[31m[System] Boot Error: %v\033[0m", err))
	}

	// Start bar re-render ticker
	// 250ms provides responsive updates while limiting CPU usage
	s.barTicker = time.NewTicker(250 * time.Millisecond)

	// Start event loop
	go s.processEvents(ctx)

	// Block on UI
	return s.ui.Run()
}

// processEvents is the main event loop.
func (s *Session) processEvents(ctx context.Context) {
	for {
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
		// A server line ends the overlay prompt
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
		// Commit current prompt to history before sending input
		if s.lastPrompt != "" {
			s.ui.Print(s.lastPrompt)
			s.lastPrompt = ""
			s.ui.SetPrompt("")
		}
		// Add non-empty input to history
		if payload != "" {
			s.history.Add(payload)
		}
		s.engine.OnInput(payload)
		// Local echo to scrollback (styled in UI)
		if le, ok := s.net.(interface{ LocalEchoEnabled() bool }); !ok || le.LocalEchoEnabled() {
			s.ui.Echo(payload)
		}

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
	if err := s.engine.Init(); err != nil {
		return err
	}

	// Set config directory
	setupCode := fmt.Sprintf("rune.config_dir = [[%s]]", s.config.ConfigDir)
	if err := s.engine.DoString("boot_config", setupCode); err != nil {
		return err
	}

	// Load Core Scripts
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

	// Load user init.lua
	initPath := filepath.Join(s.config.ConfigDir, "init.lua")
	if _, err := os.Stat(initPath); err == nil {
		if err := s.engine.DoFile(initPath); err != nil {
			return fmt.Errorf("init.lua: %w", err)
		}
	}

	// Load CLI scripts
	for _, path := range s.config.UserScripts {
		if err := s.engine.DoFile(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	s.engine.CallHook("ready")

	// Push initial state to UI after all scripts loaded
	s.pushBindsAndLayout()
	s.pushBarUpdates()

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
// Called periodically by the bar ticker.
func (s *Session) pushBarUpdates() {
	// Get current width from client state
	width := s.clientState.Width
	if width <= 0 {
		width = 80 // Default width until first resize
	}

	// Render bars and push to UI
	content := s.renderBars(width)
	if content != nil {
		s.ui.UpdateBars(content)
	}
}

// pushBindsAndLayout pushes current bindings and layout config to UI.
// Called after scripts load or reload.
func (s *Session) pushBindsAndLayout() {
	// Push bound keys
	keys := s.engine.GetBoundKeys()
	bindsMap := make(map[string]bool, len(keys))
	for _, key := range keys {
		bindsMap[key] = true
	}
	s.ui.UpdateBinds(bindsMap)

	// Push layout configuration
	luaLayout := s.engine.GetLayout()
	if len(luaLayout.Top) > 0 || len(luaLayout.Bottom) > 0 {
		s.ui.UpdateLayout(luaLayout.Top, luaLayout.Bottom)
	}
}

// handleUIMessage processes messages from the UI.
// Called when UI sends ExecuteBindMsg, WindowSizeChangedMsg, etc.
func (s *Session) handleUIMessage(msg ui.UIEvent) {
	switch m := msg.(type) {
	case ui.ExecuteBindMsg:
		s.handleKeyBind(string(m))
	case ui.WindowSizeChangedMsg:
		s.clientState.Width = m.Width
		s.clientState.Height = m.Height
		s.engine.UpdateState(s.clientState)
		// Immediately re-render bars with new width
		s.pushBarUpdates()
	case ui.ScrollStateChangedMsg:
		s.clientState.ScrollMode = m.Mode
		s.clientState.ScrollLines = m.NewLines
		s.engine.UpdateState(s.clientState)
		// Immediately re-render bars to show scroll state
		s.pushBarUpdates()
	case ui.PickerSelectMsg:
		s.executePickerCallback(m.CallbackID, m.Value, m.Accepted)
	case ui.InputChangedMsg:
		s.currentInput = string(m)
	}
}
