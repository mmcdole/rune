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

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/timer"
	"github.com/mmcdole/rune/ui"
)

var _ lua.Host = (*Session)(nil)

// Network is the connection layer Session drives. In production this
// is *network.TCPClient; tests substitute a mock so the event loop
// can be exercised without sockets.
type Network interface {
	BeginConnect(connectionID uint64)
	Connect(ctx context.Context, address string, connectionID uint64) error
	Disconnect()
	SendLine(connectionID uint64, line string) error
	SendFrame(connectionID uint64, frame []byte) error
	Inbound() <-chan network.Inbound
}

var _ Network = (*network.TCPClient)(nil)

// Config holds startup settings for a Session.
type Config struct {
	CoreScripts   embed.FS // Embedded core Lua scripts
	ConfigDir     string   // Directory for all of Rune's files (init.lua, store.json, worlds, logs)
	ConnectTarget string   // CLI connect target (world, host port, or address)
}

// Session owns application and Lua state. Its event loop serializes all access
// to that state, while network, UI, timer, and background I/O remain adapters.
// Session implements lua.Host so scripts can request those adapter operations.
type Session struct {
	// Dependencies and configuration
	net           Network
	ui            ui.UI
	timer         *timer.Service
	config        Config
	connectTarget string // CLI connect target; consumed on first boot only

	// Lua runtime
	engine        *lua.Engine
	luaGeneration uint64
	sessionStore  map[string]string // Reload-surviving state; see lua_session.go

	// Event sources and background-work lifetime
	// internalEvents carries typed outcomes from Session-owned asynchronous
	// work back to the event loop. Only the event loop applies them to Session
	// or Lua state.
	internalEvents       chan internalEvent
	timerEvents          chan timer.Event
	barTicker            *time.Ticker
	backgroundCtx        context.Context
	cancelBackgroundWork context.CancelFunc

	// Client and connection state
	clientState  lua.ClientState
	connectionID uint64            // Rejects work from previous connections
	protocol     *network.Protocol // Session-confined Telnet state

	// Input and history
	currentInput   string // Tracked so Lua can query via rune.input.get()
	currentCursor  int    // Zero-based UTF-8 byte offset exposed to Lua
	historyEntries []input.Submission
	historyLimit   int

	// Inbound text and its display lifecycle
	serverLine         serverLineBuffer
	prompt             promptOverlay
	activeNetworkBatch *networkBatchState

	// Durable state and active log
	store        map[string]json.RawMessage
	storePath    string
	storeLoadErr error // corrupt/unreadable store.json, reported at boot
	logFile      *os.File
	logPath      string
}

// networkBatchState holds decisions that must wait until Session has
// applied every effect from one network.EventBatch. Session stores the active
// state only so a re-entrant Send can defer its visual line finish until the
// batch's prompt callbacks have installed their final rewrite or gag.
type networkBatchState struct {
	connectionID               uint64
	checkForPartialPromptAtEnd bool
	lineFinishPending          bool
}

// New creates a new Session. It is passive - no goroutines start here.
func New(net Network, uiInstance ui.UI, cfg Config) *Session {
	timerEvents := make(chan timer.Event, 256)
	backgroundCtx, cancelBackgroundWork := context.WithCancel(context.Background())

	s := &Session{
		net:                  net,
		ui:                   uiInstance,
		timer:                timer.NewService(timerEvents),
		timerEvents:          timerEvents,
		internalEvents:       make(chan internalEvent, 256),
		backgroundCtx:        backgroundCtx,
		cancelBackgroundWork: cancelBackgroundWork,
		config:               cfg,
		historyEntries:       make([]input.Submission, 0, 10000),
		historyLimit:         10000,
		sessionStore:         make(map[string]string),
		prompt:               newPromptOverlay(uiInstance),
	}
	s.protocol = network.NewProtocol(0, 0)

	s.engine = lua.NewEngine(s)
	s.clientState.ScrollMode = "live"
	s.connectTarget = cfg.ConnectTarget
	s.loadStore()

	return s
}

// Run starts the session and blocks until exit.
func (s *Session) Run(ctx context.Context) error {
	// The event loop and its asynchronous producers share the caller's lifetime.
	s.stopBackgroundWork()
	ctx, cancel := context.WithCancel(ctx)
	s.backgroundCtx = ctx
	s.cancelBackgroundWork = cancel

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

// processEvents is the complete inventory of work that may mutate Session or
// call Lua. UI events share one FIFO lane; ordering between independent lanes
// is intentionally unspecified.
func (s *Session) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-s.internalEvents:
			s.handleInternalEvent(event)
		case inbound := <-s.net.Inbound():
			s.handleInbound(inbound)
		case evt := <-s.timerEvents:
			s.engine.OnTimer(evt.ID)
		case <-s.barTicker.C:
			s.pushBarUpdates()
		case event := <-s.ui.Events():
			s.handleUIEvent(event)
		}
	}
}

// handleInbound rejects stale network work and routes the current connection's
// messages.
func (s *Session) handleInbound(inbound network.Inbound) {
	if inbound.ConnectionID != s.connectionID {
		return
	}

	switch inbound.Kind {
	case network.InboundDisconnect:
		s.Disconnect()
	case network.InboundEvents:
		s.handleNetworkEventBatch(inbound.ConnectionID, inbound.Batch)
	}
}

// handleNetworkEventBatch synchronously applies one parser-produced network
// batch, including any Lua callbacks caused by its Protocol effects.
func (s *Session) handleNetworkEventBatch(connectionID uint64, batch network.EventBatch) {
	state := &networkBatchState{connectionID: connectionID}
	s.activeNetworkBatch = state
	defer s.endNetworkEventBatch(state)

	applyEffect := func(effect network.Effect) bool {
		s.applyProtocolEffect(state, effect)
		return s.connectionID == state.connectionID
	}
	processedAll := s.protocol.Process(batch, applyEffect)
	if !processedAll || !state.checkForPartialPromptAtEnd {
		return
	}

	if partial := s.serverLine.peekPartial(); partial != "" {
		s.handlePromptObservation(partial, false)
	}
}

// endNetworkEventBatch closes the scope visible to re-entrant Send calls and
// applies an accepted send's pending display boundary to the same connection.
func (s *Session) endNetworkEventBatch(state *networkBatchState) {
	if s.activeNetworkBatch == state {
		s.activeNetworkBatch = nil
	}
	if s.connectionID == state.connectionID && state.lineFinishPending {
		s.finishCurrentLine()
	}
}

// applyProtocolEffect performs one synchronous consequence of a Telnet event.
// The Process callback decides separately whether the connection is still
// current and the batch may continue.
func (s *Session) applyProtocolEffect(state *networkBatchState, effect network.Effect) {
	switch effect.Kind {
	case network.EffectSendFrame:
		if err := s.net.SendFrame(state.connectionID, effect.Data); err != nil {
			s.Disconnect()
		}
	case network.EffectServerData:
		state.checkForPartialPromptAtEnd = true
		s.handleServerData(effect.Data, state.connectionID)
	case network.EffectGA, network.EffectEOR:
		state.checkForPartialPromptAtEnd = false
		s.handlePromptBoundary()
	case network.EffectGMCPEnabled:
		s.engine.OnGMCPEnabled()
	case network.EffectGMCPMessage:
		s.engine.OnGMCP(effect.Package, effect.Payload)
	}
}

// handleServerData extends the one mutable server line. It processes complete
// lines immediately and leaves any incomplete tail for a provisional prompt
// observation at batch end.
func (s *Session) handleServerData(payload []byte, connectionID uint64) {
	lines := s.serverLine.appendData(payload)
	for _, line := range lines {
		s.handleServerLine(line)
		if s.connectionID != connectionID {
			return
		}
	}
}

// handlePromptBoundary interprets a Telnet GA/EOR marker by confirming and
// consuming the current incomplete server line.
func (s *Session) handlePromptBoundary() {
	payload := s.serverLine.consumeAtRecordMark()
	if payload != "" {
		s.handlePromptObservation(payload, true)
	}
}

// handleServerLine processes a complete server line.
func (s *Session) handleServerLine(payload string) {
	// A complete line replaces a partial line, but follows a confirmed prompt.
	s.prompt.beforeLine()

	connectionID := s.connectionID
	line := text.NewLine(payload)
	modified, show := s.engine.OnOutput(line)
	if s.connectionID != connectionID {
		// A later trigger in the same dispatch may have opened span state after
		// the connection-changing hook discarded it. Nothing from the old line
		// may survive into the replacement connection.
		s.engine.DiscardSpans()
		return
	}
	if show {
		// Display egress owns terminal safety: strip everything but
		// SGR so server clear/cursor sequences cannot wipe UI chrome
		// (issue #69). Lua hooks above saw the raw line.
		s.ui.Print(text.SanitizeDisplay(modified))
	}
}

// handlePromptObservation runs prompt hooks and replaces the prompt overlay.
// Partial lines replace each other; confirmed prompts are committed before
// later server text.
func (s *Session) handlePromptObservation(payload string, confirmed bool) {
	connectionID := s.connectionID
	s.prompt.beforeUpdate()
	if confirmed {
		s.engine.FlushSpans()
		if s.connectionID != connectionID {
			return
		}
	}

	line := text.NewLine(payload)
	modified := s.engine.OnPrompt(line, confirmed)
	// A prompt hook may replace the connection before it returns.
	if s.connectionID != connectionID {
		return
	}
	modified = text.SanitizeDisplay(modified)
	s.prompt.replace(modified, confirmed)
}

// finishCurrentLine discards the raw server tail and commits its already
// processed prompt overlay before local output or an accepted game send.
// Without an active overlay it leaves unrelated spans unchanged.
func (s *Session) finishCurrentLine() {
	s.serverLine.discard()
	if s.prompt.commit() {
		s.engine.FlushSpans()
	}
}

// handleUIEvent applies one accepted UI action or state observation.
func (s *Session) handleUIEvent(event ui.UIEvent) {
	switch event := event.(type) {
	case ui.SubmissionMsg:
		// The UI has accepted this immutable snapshot and cleared its editor.
		// Mirror that change before input hooks inspect rune.input state.
		s.currentInput = ""
		s.currentCursor = 0
		s.engine.OnInputChanged("")
		s.handleSubmission(event.Submission)
	case ui.ExecuteBindMsg:
		s.handleKeyBind(string(event))
	case ui.WindowSizeChangedMsg:
		s.clientState.Width = event.Width
		s.clientState.Height = event.Height
		if frame := s.protocol.SetWindowSize(event.Width, event.Height); len(frame) > 0 {
			_ = s.net.SendFrame(s.connectionID, frame)
		}
		s.engine.UpdateState(s.clientState)
		s.pushBarUpdates()
	case ui.ScrollStateChangedMsg:
		s.clientState.ScrollMode = event.Mode
		s.clientState.ScrollLines = event.NewLines
		s.engine.UpdateState(s.clientState)
		s.pushBarUpdates()
	case ui.SearchStateChangedMsg:
		s.clientState.SearchActive = bool(event)
		s.engine.UpdateState(s.clientState)
		s.pushBarUpdates()
	case ui.PickerSelectMsg:
		s.handlePickerResult(event.CallbackID, event.Value, event.Accepted)
	case ui.InputChangedMsg:
		s.currentInput = event.Text
		s.currentCursor = input.RuneCursorToByte(event.Text, event.Cursor)
		s.engine.OnInputChanged(event.Text)
	case ui.CursorMovedMsg:
		s.currentCursor = input.RuneCursorToByte(s.currentInput, event.Cursor)
	}
}

func (s *Session) handleSubmission(submission input.Submission) {
	// Submission is a display boundary regardless of local echo, aliases,
	// slash commands, input-hook consumption, connection state, or send result.
	s.finishCurrentLine()
	s.addHistorySubmission(submission)
	if s.protocol.LocalEchoEnabled() {
		for _, line := range submission.PhysicalLines() {
			if styled, show := s.engine.OnEcho(line); show {
				s.ui.Echo(styled)
			}
		}
	}

	s.engine.OnSubmission(submission)
}

// handleKeyBind executes a Lua key binding on the Session goroutine.
func (s *Session) handleKeyBind(key string) {
	s.engine.HandleKeyBind(key)
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
	s.engine.OnReady()
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
	s.luaGeneration++
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
