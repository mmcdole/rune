package session

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/drake/rune/lua"
	"github.com/drake/rune/mud"
	"github.com/drake/rune/timer"
)

// Ensure Session implements lua.Host at compile time
var _ lua.Host = (*Session)(nil)

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
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net mud.Network, ui mud.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 1024)

	s := &Session{
		net:         net,
		ui:          ui,
		timer:       timer.NewService(timerEvents),
		timerEvents: timerEvents,
		events:      make(chan mud.Event, 4096),
		config:      cfg,
		done:        make(chan struct{}),
	}

	s.engine = lua.NewEngine(s)

	return s
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
			s.handleEvent(event)
		case event := <-s.net.Output():
			s.handleEvent(event)
		case line := <-s.ui.Input():
			s.handleEvent(mud.Event{Type: mud.EventUserInput, Payload: line})
		case evt := <-s.timerEvents:
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

	case mud.EventDisplayLine:
		s.ui.RenderDisplayLine(event.Payload)

	case mud.EventDisplayEcho:
		s.ui.RenderEcho(event.Payload)

	case mud.EventDisplayPrompt:
		s.ui.RenderPrompt(event.Payload)
		s.lastPrompt = event.Payload

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
	s.events <- mud.Event{Type: mud.EventDisplayLine, Payload: text}
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

// Load enqueues a request to load a Lua script on the session loop.
func (s *Session) Load(path string) {
	s.events <- mud.Event{
		Type: mud.EventSystemControl,
		Control: mud.ControlOp{
			Action:     mud.ActionLoadScript,
			ScriptPath: path,
		},
	}
}

func (s *Session) Reload() {
	s.engine.CallHook("reloading")
	s.events <- mud.Event{
		Type: mud.EventAsyncResult,
		Callback: func() {
			if err := s.boot(); err != nil {
				s.ui.Render(fmt.Sprintf("\033[31mReload Failed: %v\033[0m", err))
			} else {
				s.engine.CallHook("reloaded")
			}
		},
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
