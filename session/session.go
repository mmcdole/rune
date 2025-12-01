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
	"github.com/drake/rune/ui/layout"
)

// Ensure Session implements lua.Host at compile time
var _ lua.Host = (*Session)(nil)

// Ensure Session implements layout.Provider at compile time
var _ layout.Provider = (*Session)(nil)

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
	pushUI ui.PushUI // Optional push-capable UI (nil for ConsoleUI)
	engine *lua.Engine
	timer  *timer.Service

	// Channels
	events      chan mud.Event
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

	// Input history (owned by Session, pushed to UI for Up/Down navigation)
	history      []string
	historyLimit int

	// Picker callback registry
	pickerCallbacks map[string]func(string)
	nextPickerID    int
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net mud.Network, uiInstance mud.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)

	s := &Session{
		net:             net,
		ui:              uiInstance,
		timer:           timer.NewService(timerEvents),
		timerEvents:     timerEvents,
		events:          make(chan mud.Event, 256),
		config:          cfg,
		done:            make(chan struct{}),
		history:         make([]string, 0, 1000),
		historyLimit:    10000,
		pickerCallbacks: make(map[string]func(string)),
	}

	// Check if UI supports push-based updates
	if p, ok := uiInstance.(ui.PushUI); ok {
		s.pushUI = p
	}

	s.engine = lua.NewEngine(s)

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
		s.ui.Render(fmt.Sprintf("\033[31m[System] Boot Error: %v\033[0m", err))
	}

	// Start bar re-render ticker if UI supports push updates
	// 250ms provides responsive updates while limiting CPU usage
	if s.pushUI != nil {
		s.barTicker = time.NewTicker(250 * time.Millisecond)
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
	// Get channels for push-capable UI (may be nil)
	var barTickerC <-chan time.Time
	var outboundC <-chan any
	if s.pushUI != nil {
		if s.barTicker != nil {
			barTickerC = s.barTicker.C
		}
		outboundC = s.pushUI.Outbound()
	}

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
		case <-barTickerC:
			s.pushBarUpdates()
		case msg := <-outboundC:
			s.handleUIMessage(msg)
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
		// Add non-empty input to history
		if event.Payload != "" {
			s.AddToHistory(event.Payload)
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

	// Push initial state to UI after all scripts loaded
	s.pushBindsAndLayout()
	s.pushHistory()

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

// OnConfigChange is called by Engine when binds or layout change.
// Called synchronously from Lua, so we can safely push updates.
func (s *Session) OnConfigChange() {
	s.pushBindsAndLayout()
	s.pushBarUpdates() // Render new bars immediately
}

func (s *Session) Quit() { s.shutdown() }

func (s *Session) Connect(addr string) {
	s.engine.CallHook("connecting", addr)
	go func() {
		err := s.net.Connect(addr)
		s.events <- mud.Event{
			Type: mud.EventAsyncResult,
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

func (s *Session) Disconnect() {
	s.engine.CallHook("disconnecting")
	s.net.Disconnect()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.engine.UpdateState(s.clientState)
	s.engine.CallHook("disconnected")
	s.pushBarUpdates() // Immediate UI update
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

func (s *Session) SetStatus(text string)  { s.ui.SetStatus(text) }
func (s *Session) SetInfobar(text string) { s.ui.SetInfobar(text) }

func (s *Session) PaneOp(op, name, data string) {
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

// ShowPicker displays a generic picker overlay.
// Called from Lua via rune.ui.picker.show().
// inline: if true, picker filters based on input; if false, picker captures keyboard.
func (s *Session) ShowPicker(title string, items []lua.PickerItem, onSelect func(string), inline bool) {
	if s.pushUI == nil {
		return
	}

	// Generate unique callback ID
	s.nextPickerID++
	id := fmt.Sprintf("p%d", s.nextPickerID)

	// Store callback
	s.pickerCallbacks[id] = onSelect

	// Convert lua.PickerItem to ui.GenericItem
	uiItems := make([]ui.GenericItem, len(items))
	for i, item := range items {
		uiItems[i] = ui.GenericItem{
			Text:        item.Text,
			Description: item.Description,
			Value:       item.Value,
			MatchDesc:   item.MatchDesc,
		}
	}

	// Push to UI
	s.pushUI.ShowPicker(title, uiItems, id, inline)
}

// GetHistory returns the input history for Lua.
func (s *Session) GetHistory() []string {
	// Return a copy to prevent modification
	result := make([]string, len(s.history))
	copy(result, s.history)
	return result
}

// AddToHistory adds a command to history.
func (s *Session) AddToHistory(cmd string) {
	if cmd == "" {
		return
	}
	// Don't add duplicates of the last command
	if len(s.history) > 0 && s.history[len(s.history)-1] == cmd {
		return
	}
	s.history = append(s.history, cmd)
	// Trim if over limit
	if len(s.history) > s.historyLimit {
		s.history = s.history[len(s.history)-s.historyLimit:]
	}
	s.pushHistory()
}

// SetInput sets the input line content.
func (s *Session) SetInput(text string) {
	if s.pushUI != nil {
		s.pushUI.SetInput(text)
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

// GetClientState returns the current client state for Lua.
func (s *Session) GetClientState() lua.ClientState {
	return s.clientState
}

// --- LayoutProvider Implementation ---

// Layout returns the current layout configuration from Lua.
func (s *Session) Layout() layout.Config {
	luaLayout := s.engine.GetLayout()
	return layout.Config{
		Top:    luaLayout.Top,
		Bottom: luaLayout.Bottom,
	}
}

// Bar returns the bar definition for a name, or nil if not found.
// Returns nil - Lua bars are rendered via RenderBars() instead.
func (s *Session) Bar(name string) *layout.BarDef {
	return nil
}

// Pane returns the pane definition for a name, or nil if not found.
func (s *Session) Pane(name string) *layout.PaneDef {
	return nil
}

// PaneLines returns the current buffer contents for a pane.
func (s *Session) PaneLines(name string) []string {
	return nil
}

// State returns the current client state for bar rendering.
func (s *Session) State() layout.ClientState {
	return layout.ClientState{
		Connected:   s.clientState.Connected,
		Address:     s.clientState.Address,
		ScrollMode:  s.clientState.ScrollMode,
		ScrollLines: s.clientState.ScrollLines,
	}
}

// renderBars calls all Lua bar renderers and returns their content.
// Must be called from Session goroutine (thread-safe Lua access).
// Converts lua.BarData to layout.BarContent (decoupling lua from ui).
func (s *Session) renderBars(width int) map[string]layout.BarContent {
	names := s.engine.GetBarNames()
	if len(names) == 0 {
		return nil
	}

	result := make(map[string]layout.BarContent, len(names))
	for _, name := range names {
		if data, ok := s.engine.RenderBar(name, width); ok {
			result[name] = layout.BarContent{
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
	if s.pushUI == nil {
		return
	}

	// Get current width from client state
	width := s.clientState.Width
	if width <= 0 {
		width = 80 // Default width until first resize
	}

	// Render bars and push to UI
	content := s.renderBars(width)
	if content != nil {
		s.pushUI.UpdateBars(content)
	}
}

// pushBindsAndLayout pushes current bindings and layout config to UI.
// Called after scripts load or reload.
func (s *Session) pushBindsAndLayout() {
	if s.pushUI == nil {
		return
	}

	// Push bound keys
	keys := s.engine.GetBoundKeys()
	bindsMap := make(map[string]bool, len(keys))
	for _, key := range keys {
		bindsMap[key] = true
	}
	s.pushUI.UpdateBinds(bindsMap)

	// Push layout configuration
	luaLayout := s.engine.GetLayout()
	if len(luaLayout.Top) > 0 || len(luaLayout.Bottom) > 0 {
		s.pushUI.UpdateLayout(luaLayout.Top, luaLayout.Bottom)
	}
}

// pushHistory pushes input history to UI for Up/Down navigation.
func (s *Session) pushHistory() {
	if s.pushUI != nil {
		s.pushUI.UpdateHistory(s.history)
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
		// Look up and execute the callback
		if cb, ok := s.pickerCallbacks[m.CallbackID]; ok {
			delete(s.pickerCallbacks, m.CallbackID) // One-shot
			if m.Accepted && cb != nil {
				cb(m.Value)
			}
		}
	}
}
