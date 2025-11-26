package session

import (
	"embed"
	"fmt"
	"os"

	"github.com/drake/rune/engine"
	"github.com/drake/rune/internal/buffer"
	"github.com/drake/rune/mud"
)

// Config holds session configuration
type Config struct {
	CoreScripts embed.FS
	ConfigDir   string
	UserScripts []string
}

// Session orchestrates the MUD client components.
// It implements engine.Host to bridge the LuaEngine with the rest of the system.
type Session struct {
	// Dependencies (injected)
	network mud.Network
	ui      mud.UI

	// Owned components
	engine *engine.LuaEngine

	// Internal channels
	eventsIn  chan<- mud.Event
	eventsOut <-chan mud.Event
	netOut    chan string
	uiIn      chan<- string
	uiOut     <-chan string

	// Config
	coreScripts embed.FS
	configDir   string
	userScripts []string
}

// New creates a new Session with the given dependencies.
func New(network mud.Network, ui mud.UI, cfg Config) *Session {
	// Create channels (unbounded buffers)
	eventsIn, eventsOut := buffer.Unbounded[mud.Event](100, 50000)
	netOut := make(chan string, 100)
	uiIn, uiOut := buffer.Unbounded[string](100, 50000)

	s := &Session{
		network:     network,
		ui:          ui,
		eventsIn:    eventsIn,
		eventsOut:   eventsOut,
		netOut:      netOut,
		uiIn:        uiIn,
		uiOut:       uiOut,
		coreScripts: cfg.CoreScripts,
		configDir:   cfg.ConfigDir,
		userScripts: cfg.UserScripts,
	}

	// Create engine with Session as Host
	s.engine = engine.NewLuaEngine(s)

	return s
}

// Run starts the session and blocks until the UI exits.
func (s *Session) Run() error {
	defer s.engine.Close()

	// Spawn bridge goroutines
	go s.bridgeNetworkToEvents()
	go s.bridgeUIToEvents()
	go s.runNetworkSender()
	go s.runUIRenderer()

	// Initialize Lua state with core scripts
	if err := s.engine.InitState(s.coreScripts, s.configDir); err != nil {
		s.uiIn <- fmt.Sprintf("\033[31m[Error] Loading scripts: %v\033[0m", err)
	}

	// Start event loop in goroutine
	go s.runEventLoop()

	// Load user scripts from command line args
	if len(s.userScripts) > 0 {
		if err := s.engine.LoadUserScripts(s.userScripts); err != nil {
			return fmt.Errorf("loading user scripts: %w", err)
		}
	}

	// Block on UI
	return s.ui.Run()
}

// --- Bridge goroutines ---

func (s *Session) bridgeNetworkToEvents() {
	for event := range s.network.Output() {
		s.eventsIn <- event
	}
}

func (s *Session) bridgeUIToEvents() {
	for line := range s.ui.Input() {
		s.eventsIn <- mud.Event{Type: mud.EventUserInput, Payload: line}
	}
}

func (s *Session) runNetworkSender() {
	for line := range s.netOut {
		s.network.Send(line)
	}
}

func (s *Session) runUIRenderer() {
	for line := range s.uiOut {
		s.ui.Render(line)
	}
}

// --- Event loop ---

func (s *Session) runEventLoop() {
	for event := range s.eventsOut {
		switch event.Type {
		case mud.EventServerLine:
			modified, keep := s.engine.OnOutput(event.Payload)
			if keep {
				s.uiIn <- modified
			}

		case mud.EventServerPrompt:
			modified := s.engine.OnPrompt(event.Payload)
			s.ui.RenderPrompt(modified)

		case mud.EventUserInput:
			s.ui.Render(event.Payload) // Local echo
			s.engine.OnInput(event.Payload)

		case mud.EventTimer:
			if event.Callback != nil {
				s.engine.ExecuteCallback(event.Callback)
			}

		case mud.EventSystemControl:
			s.handleSystemControl(event.Control)
		}
	}
}

func (s *Session) handleSystemControl(ctrl mud.ControlOp) {
	switch ctrl.Action {
	case mud.ActionQuit:
		os.Exit(0)

	case mud.ActionConnect:
		addr := ctrl.Address
		s.engine.CallHook("connecting", addr)
		go func() {
			if err := s.network.Connect(addr); err != nil {
				s.eventsIn <- mud.Event{
					Type: mud.EventTimer,
					Callback: func() {
						s.engine.CallHook("error", "Connection failed: "+err.Error())
					},
				}
			} else {
				s.eventsIn <- mud.Event{
					Type: mud.EventTimer,
					Callback: func() {
						s.engine.CallHook("connected", addr)
					},
				}
			}
		}()

	case mud.ActionDisconnect:
		s.engine.CallHook("disconnecting")
		s.network.Disconnect()
		s.engine.CallHook("disconnected")

	case mud.ActionReload:
		s.engine.CallHook("reloading")
		if err := s.engine.InitState(s.coreScripts, s.configDir); err != nil {
			s.engine.CallHook("error", err.Error())
			return
		}
		s.engine.CallHook("reloaded")

	case mud.ActionLoad:
		path := ctrl.ScriptPath
		if err := s.engine.LoadUserScripts([]string{path}); err != nil {
			s.engine.CallHook("error", err.Error())
		} else {
			s.engine.CallHook("loaded", path)
		}
	}
}

// --- Host interface implementation ---

func (s *Session) SendToNetwork(data string) {
	s.netOut <- data
}

func (s *Session) SendToDisplay(text string) {
	s.uiIn <- text
}

func (s *Session) RequestQuit() {
	s.eventsIn <- mud.Event{
		Type:    mud.EventSystemControl,
		Control: mud.ControlOp{Action: mud.ActionQuit},
	}
}

func (s *Session) RequestConnect(address string) {
	s.eventsIn <- mud.Event{
		Type:    mud.EventSystemControl,
		Control: mud.ControlOp{Action: mud.ActionConnect, Address: address},
	}
}

func (s *Session) RequestDisconnect() {
	s.eventsIn <- mud.Event{
		Type:    mud.EventSystemControl,
		Control: mud.ControlOp{Action: mud.ActionDisconnect},
	}
}

func (s *Session) RequestReload() {
	s.eventsIn <- mud.Event{
		Type:    mud.EventSystemControl,
		Control: mud.ControlOp{Action: mud.ActionReload},
	}
}

func (s *Session) RequestLoad(scriptPath string) {
	s.eventsIn <- mud.Event{
		Type:    mud.EventSystemControl,
		Control: mud.ControlOp{Action: mud.ActionLoad, ScriptPath: scriptPath},
	}
}

func (s *Session) SetStatus(text string) {
	s.ui.SetStatus(text)
}

func (s *Session) SetInfobar(text string) {
	s.ui.SetInfobar(text)
}

func (s *Session) CreatePane(name string) {
	s.ui.CreatePane(name)
}

func (s *Session) WritePane(name, text string) {
	s.ui.WritePane(name, text)
}

func (s *Session) TogglePane(name string) {
	s.ui.TogglePane(name)
}

func (s *Session) ClearPane(name string) {
	s.ui.ClearPane(name)
}

func (s *Session) BindPaneKey(key, name string) {
	s.ui.BindPaneKey(key, name)
}

func (s *Session) SendTimerEvent(callback func()) {
	s.eventsIn <- mud.Event{Type: mud.EventTimer, Callback: callback}
}
