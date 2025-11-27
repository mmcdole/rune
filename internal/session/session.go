package session

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/drake/rune/internal/timer"
	"github.com/drake/rune/lua"
	"github.com/drake/rune/mud"
)

// Config holds session configuration
type Config struct {
	CoreScripts embed.FS // Embedded core Lua scripts
	ConfigDir   string   // Path to ~/.config/rune
	UserScripts []string // CLI script arguments
}

// Session orchestrates the MUD client components.
type Session struct {
	// Components
	net       mud.Network
	ui        mud.UI
	engine    *lua.Engine
	scheduler *timer.Scheduler

	// Channels
	events   chan mud.Event
	timerOut chan func()

	// Timer cancellation - Session owns Go timers, Engine owns callbacks
	timerCancels map[int]func()

	// Track last prompt overlay to commit to history when replaced
	lastPrompt string

	// Config (retained for reload)
	config Config
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net mud.Network, ui mud.UI, cfg Config) *Session {
	timerOut := make(chan func(), 1024)

	s := &Session{
		net:          net,
		ui:           ui,
		scheduler:    timer.New(timerOut),
		timerOut:     timerOut,
		events:       make(chan mud.Event, 4096),
		timerCancels: make(map[int]func()),
		config:       cfg,
	}

	s.engine = lua.NewEngine(s)

	return s
}

// Run starts the session and blocks until exit.
func (s *Session) Run() error {
	defer s.engine.Close()

	// Network -> Events
	go func() {
		for evt := range s.net.Output() {
			s.events <- evt
		}
	}()

	// UI -> Events
	go func() {
		for line := range s.ui.Input() {
			s.events <- mud.Event{Type: mud.EventUserInput, Payload: line}
		}
	}()

	// Timer -> Events
	go func() {
		for cb := range s.timerOut {
			s.events <- mud.Event{Type: mud.EventTimer, Callback: cb}
		}
	}()

	// Boot the system
	if err := s.boot(); err != nil {
		s.ui.Render(fmt.Sprintf("\033[31m[System] Boot Error: %v\033[0m", err))
	}

	// Start event loop
	go s.processEvents()

	// Block on UI
	return s.ui.Run()
}

// processEvents is the main event loop.
func (s *Session) processEvents() {
	for event := range s.events {
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
				s.events <- mud.Event{Type: mud.EventDisplayEcho, Payload: event.Payload}
			}

		case mud.EventTimer:
			if event.Callback != nil {
				event.Callback()
			}

		case mud.EventSystemControl:
			s.handleControl(event.Control)
		}
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
		os.Exit(0)
	case mud.ActionConnect:
		s.Connect(ctrl.Address)
	case mud.ActionDisconnect:
		s.Disconnect()
	case mud.ActionReload:
		s.Reload()
	}
}

// --- Host Implementation ---

func (s *Session) Print(text string) {
	s.events <- mud.Event{Type: mud.EventDisplayLine, Payload: text}
}
func (s *Session) Send(data string) { s.net.Send(data) }

func (s *Session) Quit() {
	os.Exit(0)
}

func (s *Session) Connect(addr string) {
	s.engine.CallHook("connecting", addr)
	go func() {
		err := s.net.Connect(addr)
		s.events <- mud.Event{
			Type: mud.EventTimer,
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

func (s *Session) Reload() {
	s.engine.CallHook("reloading")
	s.events <- mud.Event{
		Type: mud.EventTimer,
		Callback: func() {
			if err := s.boot(); err != nil {
				s.ui.Render(fmt.Sprintf("\033[31mReload Failed: %v\033[0m", err))
			} else {
				s.engine.CallHook("reloaded")
			}
		},
	}
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

// ScheduleTimer schedules a timer wake-up call.
func (s *Session) ScheduleTimer(id int, d time.Duration) {
	cancel := s.scheduler.Schedule(d, func() {
		s.events <- mud.Event{
			Type:     mud.EventTimer,
			Callback: func() { s.engine.OnTimer(id) },
		}
	})
	s.timerCancels[id] = cancel
}

// CancelTimer implements Host - cancels a scheduled wake-up.
func (s *Session) CancelTimer(id int) {
	if cancel, ok := s.timerCancels[id]; ok {
		cancel()
		delete(s.timerCancels, id)
	}
}

// CancelAllTimers implements Host - cancels all scheduled wake-ups.
func (s *Session) CancelAllTimers() {
	for id, cancel := range s.timerCancels {
		cancel()
		delete(s.timerCancels, id)
	}
}
