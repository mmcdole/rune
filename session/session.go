package session

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
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
	Connect(address string) error
	Disconnect()
	Send(data string)
	Output() <-chan event.Event
}

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
	engine  *lua.Engine
	adapter *LuaAdapter

	// Managers
	history   *HistoryManager
	callbacks *CallbackManager

	// Channels
	events      chan event.Event
	timerEvents chan timer.Event
	barTicker   *time.Ticker // Periodic bar re-render ticker

	// Track last prompt overlay to commit to history when replaced
	lastPrompt string

	// Config (retained for reload)
	config Config

	// Shutdown coordination
	done      chan struct{}
	closeOnce sync.Once

	// Stats (atomic for lock-free reads)
	eventsProcessed atomic.Uint64

	// Client state (for Lua rune.state access)
	clientState lua.ClientState

	// Current input content (tracked for rune.input.get())
	currentInput string
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net Network, uiInstance ui.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:         net,
		ui:          uiInstance,
		timer:       timer.NewService(timerEvents),
		timerEvents: timerEvents,
		events:      make(chan event.Event, 256),
		config:      cfg,
		done:        make(chan struct{}),
		history:     NewHistoryManager(10000),
		callbacks:   NewCallbackManager(),
	}


	// Create adapter (bridges Lua services to Session infrastructure)
	s.adapter = NewLuaAdapter(s)

	// Create engine with segregated service interfaces
	// LuaAdapter implements all 6 interfaces, so we pass it for each
	s.engine = lua.NewEngine(s.adapter, s.adapter, s.adapter, s.adapter, s.adapter, s.adapter)

	// Initialize client state defaults
	s.clientState.ScrollMode = "live"

	return s
}

// Stats holds session statistics for monitoring.
type Stats struct {
	EventsProcessed uint64
	EventQueueLen   int
	EventQueueCap   int
	TimerQueueLen   int
	TimerQueueCap   int
	Goroutines      int
	Lua             lua.Stats
	Timer           timer.Stats
	Network         network.Stats
}

// Stats returns current session and component statistics.
func (s *Session) Stats() Stats {
	var netStats network.Stats
	if tc, ok := s.net.(*network.TCPClient); ok {
		netStats = tc.Stats()
	}

	return Stats{
		EventsProcessed: s.eventsProcessed.Load(),
		EventQueueLen:   len(s.events),
		EventQueueCap:   cap(s.events),
		TimerQueueLen:   len(s.timerEvents),
		TimerQueueCap:   cap(s.timerEvents),
		Goroutines:      runtime.NumGoroutine(),
		Lua:             s.engine.Stats(),
		Timer:           s.timer.Stats(),
		Network:         netStats,
	}
}

// Run starts the session and blocks until exit.
func (s *Session) Run() error {
	defer s.engine.Close()

	// Boot the system
	if err := s.boot(); err != nil {
		s.ui.Print(fmt.Sprintf("\033[31m[System] Boot Error: %v\033[0m", err))
	}

	// Start bar re-render ticker
	// 250ms provides responsive updates while limiting CPU usage
	s.barTicker = time.NewTicker(250 * time.Millisecond)

	// Start event loop
	go s.processEvents()

	// Block on UI
	err := s.ui.Run()
	// Ensure shutdown of goroutines/resources when UI exits
	s.shutdown()
	return err
}

// processEvents is the main event loop.
func (s *Session) processEvents() {
	for {
		select {
		case <-s.done:
			return
		case event := <-s.events:
			s.eventsProcessed.Add(1)
			s.handleEvent(event)
		case event := <-s.net.Output():
			s.eventsProcessed.Add(1)
			s.handleEvent(event)
		case line := <-s.ui.Input():
			s.eventsProcessed.Add(1)
			s.handleEvent(event.Event{Type: event.UserInput, Payload: line})
		case evt := <-s.timerEvents:
			s.eventsProcessed.Add(1)
			s.engine.OnTimer(evt.ID, evt.Repeating)
		case <-s.barTicker.C:
			s.pushBarUpdates()
		case msg := <-s.ui.Outbound():
			s.handleUIMessage(msg)
		}
	}
}

// handleEvent executes a single event on the session loop.
func (s *Session) handleEvent(ev event.Event) {
	switch ev.Type {
	case event.NetLine:
		line := text.NewLine(ev.Payload)
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
		line := text.NewLine(ev.Payload)
		modified := s.engine.OnPrompt(line)
		s.lastPrompt = modified
		s.ui.SetPrompt(modified)

	case event.UserInput:
		// Commit current prompt to history before sending input
		if s.lastPrompt != "" {
			s.ui.Print(s.lastPrompt)
			s.lastPrompt = ""
			s.ui.SetPrompt("")
		}
		// Add non-empty input to history
		if ev.Payload != "" {
			s.history.Add(ev.Payload)
		}
		s.engine.OnInput(ev.Payload)
		// Local echo to scrollback (styled in UI)
		if le, ok := s.net.(interface{ LocalEchoEnabled() bool }); !ok || le.LocalEchoEnabled() {
			s.ui.Echo(ev.Payload)
		}

	case event.AsyncResult:
		if ev.Callback != nil {
			ev.Callback()
		}

	case event.SystemControl:
		s.handleControl(ev.Control)
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

	return nil
}

// handleControl processes system control events.
func (s *Session) handleControl(ctrl event.ControlOp) {
	switch ctrl.Action {
	case event.ActionQuit:
		s.shutdown()
	case event.ActionConnect:
		s.connect(ctrl.Address)
	case event.ActionDisconnect:
		s.disconnect()
	case event.ActionReload:
		s.reload()
	case event.ActionLoadScript:
		s.loadScript(ctrl.ScriptPath)
	}
}

// --- Internal helpers for LuaAdapter ---

// connect handles connection logic (called by adapter and handleControl).
func (s *Session) connect(addr string) {
	s.engine.CallHook("connecting", addr)
	go func() {
		err := s.net.Connect(addr)
		s.events <- event.Event{
			Type: event.AsyncResult,
			Callback: func() {
				if err != nil {
					s.clientState.Connected = false
					s.clientState.Address = ""
					s.engine.UpdateState(s.clientState)
					s.engine.CallHook("error", err.Error())
				} else {
					s.clientState.Connected = true
					s.clientState.Address = addr
					s.engine.UpdateState(s.clientState)
					s.engine.CallHook("connected", addr)
				}
				s.pushBarUpdates() // Immediate UI update
			},
		}
	}()
}

// disconnect handles disconnection logic (called by adapter and handleControl).
func (s *Session) disconnect() {
	s.engine.CallHook("disconnecting")
	s.net.Disconnect()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.engine.UpdateState(s.clientState)
	s.engine.CallHook("disconnected")
	s.pushBarUpdates() // Immediate UI update
}

// reload schedules VM reinitialization (called by adapter and handleControl).
// Must be deferred because it destroys the currently executing Lua state.
func (s *Session) reload() {
	s.engine.CallHook("reloading")
	select {
	case s.events <- event.Event{
		Type: event.AsyncResult,
		Callback: func() {
			if err := s.boot(); err != nil {
				s.ui.Print(fmt.Sprintf("\033[31mReload Failed: %v\033[0m", err))
			} else {
				s.engine.CallHook("reloaded")
			}
		},
	}:
	default:
		s.ui.Print("\033[31mReload Failed: event queue full\033[0m")
	}
}

// loadScript loads a Lua script file and notifies hooks (called by adapter).
func (s *Session) loadScript(path string) {
	if path == "" {
		s.ui.Print("\033[31mLoad Failed: empty path\033[0m")
		return
	}

	if err := s.engine.DoFile(path); err != nil {
		s.ui.Print(fmt.Sprintf("\033[31mLoad Failed (%s): %v\033[0m", path, err))
		return
	}

	s.engine.CallHook("loaded", path)
}

// shutdown attempts a coordinated shutdown of goroutines, timers, network, and UI.
func (s *Session) shutdown() {
	s.closeOnce.Do(func() {
		close(s.done)
		// Stop bar ticker if running
		if s.barTicker != nil {
			s.barTicker.Stop()
		}
		// Stop timers and network; request UI exit.
		s.timer.CancelAll()
		s.net.Disconnect()
		s.ui.Quit()
	})
}

// renderBars calls all Lua bar renderers and returns their content.
// Must be called from Session goroutine (thread-safe Lua access).
// Converts lua.BarData to ui.BarContent (decoupling lua from ui).
func (s *Session) renderBars(width int) map[string]ui.BarContent {
	names := s.engine.GetBarNames()
	if len(names) == 0 {
		return nil
	}

	result := make(map[string]ui.BarContent, len(names))
	for _, name := range names {
		if data, ok := s.engine.RenderBar(name, width); ok {
			result[name] = ui.BarContent{
				Left:   data.Left,
				Center: data.Center,
				Right:  data.Right,
			}
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
func (s *Session) handleUIMessage(msg any) {
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
		s.callbacks.Execute(m.CallbackID, m.Value, m.Accepted)
	case ui.InputChangedMsg:
		s.currentInput = string(m)
	}
}
