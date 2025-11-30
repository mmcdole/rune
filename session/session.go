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

	"github.com/drake/rune/lua"
	"github.com/drake/rune/mud"
	"github.com/drake/rune/network"
	"github.com/drake/rune/timer"
	"github.com/drake/rune/ui"
)

// Ensure Session implements lua.Host at compile time
var _ lua.Host = (*Session)(nil)

// Ensure Session implements ui.DataProvider at compile time
var _ ui.DataProvider = (*Session)(nil)

// Config holds session configuration
type Config struct {
	CoreScripts embed.FS // Embedded core Lua scripts
	ConfigDir   string   // Path to ~/.config/rune
	UserScripts []string // CLI script arguments
}

// Session orchestrates the MUD client components.
type Session struct {
	// Components
	net    mud.Network
	ui     mud.UI
	engine *lua.Engine
	timer  *timer.Service

	// Channels
	events      chan mud.Event
	timerEvents chan timer.Event

	// Track last prompt overlay to commit to history when replaced
	lastPrompt string

	// Config (retained for reload)
	config Config

	// Shutdown coordination
	done      chan struct{}
	closeOnce sync.Once

	// Stats (atomic for lock-free reads)
	eventsProcessed atomic.Uint64
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net mud.Network, ui mud.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:         net,
		ui:          ui,
		timer:       timer.NewService(timerEvents),
		timerEvents: timerEvents,
		events:      make(chan mud.Event, 256),
		config:      cfg,
		done:        make(chan struct{}),
	}

	s.engine = lua.NewEngine(s)

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
		s.ui.Render(fmt.Sprintf("\033[31m[System] Boot Error: %v\033[0m", err))
	}

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
			s.handleEvent(mud.Event{Type: mud.EventUserInput, Payload: line})
		case evt := <-s.timerEvents:
			s.eventsProcessed.Add(1)
			s.engine.OnTimer(evt.ID, evt.Repeating)
		}
	}
}

// handleEvent executes a single event on the session loop.
func (s *Session) handleEvent(event mud.Event) {
	switch event.Type {
	case mud.EventNetLine:
		if modified, show := s.engine.OnOutput(event.Payload); show {
			s.ui.RenderDisplayLine(modified)
		}
		// A server line ends the overlay prompt
		s.lastPrompt = ""
		s.ui.RenderPrompt("")

	case mud.EventNetPrompt:
		// Commit previous prompt to scrollback before showing new one
		if s.lastPrompt != "" {
			s.ui.RenderDisplayLine(s.lastPrompt)
		}
		modified := s.engine.OnPrompt(event.Payload)
		s.lastPrompt = modified
		s.ui.RenderPrompt(modified)

	case mud.EventUserInput:
		// Commit current prompt to history before sending input
		if s.lastPrompt != "" {
			s.ui.RenderDisplayLine(s.lastPrompt)
			s.lastPrompt = ""
			s.ui.RenderPrompt("")
		}
		s.engine.OnInput(event.Payload)
		// Local echo to scrollback (styled in UI)
		if le, ok := s.net.(interface{ LocalEchoEnabled() bool }); !ok || le.LocalEchoEnabled() {
			s.ui.RenderEcho(event.Payload)
		}

	case mud.EventAsyncResult:
		if event.Callback != nil {
			event.Callback()
		}

	case mud.EventSystemControl:
		s.handleControl(event.Control)
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
	return nil
}

// handleControl processes system control events.
func (s *Session) handleControl(ctrl mud.ControlOp) {
	switch ctrl.Action {
	case mud.ActionQuit:
		s.shutdown()
	case mud.ActionConnect:
		s.Connect(ctrl.Address)
	case mud.ActionDisconnect:
		s.Disconnect()
	case mud.ActionReload:
		s.Reload()
	case mud.ActionLoadScript:
		s.loadScript(ctrl.ScriptPath)
	}
}

// --- Host Implementation ---

func (s *Session) Print(text string) {
	// Print is called synchronously from Lua (within the event loop),
	// so render directly instead of going through the channel to avoid deadlock.
	s.ui.RenderDisplayLine(text)
}
func (s *Session) Send(data string) { s.net.Send(data) }

func (s *Session) Quit() { s.shutdown() }

func (s *Session) Connect(addr string) {
	s.engine.CallHook("connecting", addr)
	go func() {
		err := s.net.Connect(addr)
		s.events <- mud.Event{
			Type: mud.EventAsyncResult,
			Callback: func() {
				if err != nil {
					s.engine.CallHook("error", err.Error())
				} else {
					s.engine.CallHook("connected", addr)
				}
			},
		}
	}()
}

func (s *Session) Disconnect() {
	s.engine.CallHook("disconnecting")
	s.net.Disconnect()
	s.engine.CallHook("disconnected")
}

// Load loads a Lua script synchronously. Called from Lua, so executes directly.
func (s *Session) Load(path string) {
	s.loadScript(path)
}

// Reload schedules VM reinitialization. Must be deferred because it destroys the
// currently executing Lua state. Uses non-blocking send to avoid deadlock.
func (s *Session) Reload() {
	s.engine.CallHook("reloading")
	select {
	case s.events <- mud.Event{
		Type: mud.EventAsyncResult,
		Callback: func() {
			if err := s.boot(); err != nil {
				s.ui.Render(fmt.Sprintf("\033[31mReload Failed: %v\033[0m", err))
			} else {
				s.engine.CallHook("reloaded")
			}
		},
	}:
	default:
		s.ui.Render("\033[31mReload Failed: event queue full\033[0m")
	}
}

// loadScript loads a Lua script file and notifies hooks. Runs on the session goroutine.
func (s *Session) loadScript(path string) {
	if path == "" {
		s.ui.Render("\033[31mLoad Failed: empty path\033[0m")
		return
	}

	if err := s.engine.DoFile(path); err != nil {
		s.ui.Render(fmt.Sprintf("\033[31mLoad Failed (%s): %v\033[0m", path, err))
		return
	}

	s.engine.CallHook("loaded", path)
}

// shutdown attempts a coordinated shutdown of goroutines, timers, network, and UI.
func (s *Session) shutdown() {
	s.closeOnce.Do(func() {
		close(s.done)
		// Stop timers and network; request UI exit.
		s.timer.CancelAll()
		s.net.Disconnect()
		s.ui.Quit()
	})
}

func (s *Session) SetStatus(text string)  { s.ui.SetStatus(text) }
func (s *Session) SetInfobar(text string) { s.ui.SetInfobar(text) }

func (s *Session) Pane(op, name, data string) {
	switch op {
	case "create":
		s.ui.CreatePane(name)
	case "write":
		s.ui.WritePane(name, data)
	case "toggle":
		s.ui.TogglePane(name)
	case "clear":
		s.ui.ClearPane(name)
	case "bind":
		s.ui.BindPaneKey(data, name)
	}
}

// TimerAfter schedules a one-shot timer. Returns the timer ID.
func (s *Session) TimerAfter(d time.Duration) int {
	return s.timer.After(d)
}

// TimerEvery schedules a repeating timer. Returns the timer ID.
func (s *Session) TimerEvery(d time.Duration) int {
	return s.timer.Every(d)
}

// TimerCancel cancels a timer by ID.
func (s *Session) TimerCancel(id int) {
	s.timer.Cancel(id)
}

// TimerCancelAll cancels all timers.
func (s *Session) TimerCancelAll() {
	s.timer.CancelAll()
}

// --- DataProvider Implementation ---

// Commands returns all slash commands for the UI.
func (s *Session) Commands() []ui.CommandInfo {
	cmds := s.engine.GetCommands()
	result := make([]ui.CommandInfo, len(cmds))
	for i, c := range cmds {
		result[i] = ui.CommandInfo{Name: c.Name, Description: c.Description}
	}
	return result
}

// Aliases returns all aliases for the UI.
func (s *Session) Aliases() []ui.AliasInfo {
	aliases := s.engine.GetAliases()
	result := make([]ui.AliasInfo, len(aliases))
	for i, a := range aliases {
		result[i] = ui.AliasInfo{Name: a.Name, Value: a.Value}
	}
	return result
}
