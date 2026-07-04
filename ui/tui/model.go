package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// tickMsg drives the output flush: lines are batched within a 16ms
// window to prevent excessive renders on fast MUD output.
type tickMsg time.Time

// doTick returns a command that sends a tickMsg after the given duration.
func doTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Model is the main Bubble Tea model for the TUI. It routes messages
// between the session and the widgets; input-mode policy lives in the
// inputController, layout and rendering in layout.go.
type Model struct {
	// Layout
	widgets map[string]widget.Widget // all named widgets: input, separator, bars

	// Widgets
	scrollback *widget.ScrollbackBuffer
	viewport   *widget.Viewport
	input      *widget.Input
	panes      *widget.PaneManager

	// Input-mode state machine (normal / modal picker / inline picker)
	inputCtl *inputController

	// Push-based state from Session
	boundKeys  map[string]bool
	barContent map[string]ui.BarContent
	luaLayout  struct {
		Top    []ui.LayoutEntry
		Bottom []ui.LayoutEntry
	}

	// State
	lastPrompt   string
	width        int
	height       int
	inputChan    chan<- string
	outbound     chan<- ui.UIEvent
	initialized  bool
	pendingLines []string
}

// NewModel creates a new TUI model.
func NewModel(inputChan chan<- string, outbound chan<- ui.UIEvent) *Model {
	styles := style.DefaultStyles()
	scrollback := widget.NewScrollbackBuffer(100000)
	viewport := widget.NewViewport(scrollback)
	input := widget.NewInput(styles)
	panes := widget.NewPaneManager(styles)

	m := &Model{
		scrollback: scrollback,
		viewport:   viewport,
		input:      input,
		panes:      panes,
		inputChan:  inputChan,
		outbound:   outbound,
		widgets:    make(map[string]widget.Widget),
	}
	m.inputCtl = newInputController(input, m.sendOutbound, m.sendLine, m.isBound, m.handleScrollKey)

	// Register static widgets
	m.widgets["input"] = input
	m.widgets["separator"] = widget.NewSeparator()

	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		doTick(),
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// System
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tickMsg:
		return m.handleTick()
	case tea.KeyMsg:
		m.inputCtl.HandleKey(msg)
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)

	// Session config updates
	case ui.UpdateBindsMsg, ui.UpdateBarsMsg, ui.UpdateLayoutMsg:
		return m.handleConfigUpdate(msg)

	// Server output
	case ui.PrintLineMsg, ui.EchoLineMsg, ui.PromptMsg:
		return m.handleServerOutput(msg)

	// Pane operations
	case ui.PaneCreateMsg, ui.PaneWriteMsg, ui.PaneToggleMsg, ui.PaneClearMsg:
		return m.handlePaneMsg(msg)

	// Input control
	case ui.ShowPickerMsg:
		m.inputCtl.ShowPicker(msg)
		return m, nil
	case ui.SetInputMsg:
		m.inputCtl.SetText(string(msg))
		return m, nil

	// Input primitives (from Lua)
	case ui.InputSetCursorMsg:
		m.input.SetCursor(int(msg))
		return m, nil

	// Pane scrolling (from Lua)
	case ui.PaneScrollUpMsg:
		if msg.Name == "main" {
			m.viewport.ScrollUp(msg.Lines)
			m.updateScrollState()
		}
		return m, nil
	case ui.PaneScrollDownMsg:
		if msg.Name == "main" {
			m.viewport.ScrollDown(msg.Lines)
			m.updateScrollState()
		}
		return m, nil
	case ui.PaneScrollToTopMsg:
		if msg.Name == "main" {
			m.viewport.GotoTop()
			m.updateScrollState()
		}
		return m, nil
	case ui.PaneScrollToBottomMsg:
		if msg.Name == "main" {
			m.viewport.GotoBottom()
			m.updateScrollState()
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.initialized = true
	m.sendOutbound(ui.WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
	return m, nil
}

// handleTick flushes the pending line batch.
func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	m.flushPending()
	return m, doTick()
}

// flushPending appends all batched server lines to the scrollback.
func (m *Model) flushPending() {
	if len(m.pendingLines) == 0 {
		return
	}
	m.appendLines(m.pendingLines)
	m.pendingLines = nil
}

func (m *Model) handleConfigUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.UpdateBindsMsg:
		m.boundKeys = msg
	case ui.UpdateBarsMsg:
		m.syncBars(msg)
	case ui.UpdateLayoutMsg:
		m.luaLayout.Top = msg.Top
		m.luaLayout.Bottom = msg.Bottom
	}
	return m, nil
}

// syncBars updates the widgets map to match the current bar content.
// Creates new Bar instances for new names, removes stale ones, updates
// existing ones. A bar whose name collides with a built-in widget
// ("input", "separator") is ignored rather than allowed to clobber it.
func (m *Model) syncBars(content map[string]ui.BarContent) {
	// Remove bars that no longer exist in content
	for name := range m.barContent {
		if _, exists := content[name]; !exists {
			if _, isBar := m.widgets[name].(*widget.Bar); isBar {
				delete(m.widgets, name)
			}
		}
	}

	// Add or update bars
	for name, barContent := range content {
		w, exists := m.widgets[name]
		if !exists {
			w = widget.NewBar(name)
			m.widgets[name] = w
		}
		if bar, isBar := w.(*widget.Bar); isBar {
			bar.SetContent(barContent)
		}
	}

	m.barContent = content
}

func (m *Model) handleServerOutput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.PrintLineMsg:
		cleanLine := util.FilterClearSequences(string(msg))
		m.pendingLines = append(m.pendingLines, cleanLine)
	case ui.EchoLineMsg:
		// Flush batched server lines first so the echo cannot render
		// ahead of output that arrived before it.
		m.flushPending()
		m.appendLines([]string{string(msg)})
	case ui.PromptMsg:
		text := string(msg)
		if text != m.lastPrompt {
			m.viewport.SetPrompt(text)
			m.lastPrompt = text
		}
	}
	return m, nil
}

func (m *Model) handlePaneMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.PaneCreateMsg:
		m.panes.Create(msg.Name)
	case ui.PaneWriteMsg:
		m.panes.Write(msg.Name, msg.Text)
	case ui.PaneToggleMsg:
		m.panes.Toggle(msg.Name)
	case ui.PaneClearMsg:
		m.panes.Clear(msg.Name)
	}
	return m, nil
}

// wheelScrollLines is how far one mouse-wheel tick scrolls the main
// viewport. Matches the common terminal-emulator default.
const wheelScrollLines = 3

// handleMouse scrolls the main viewport on wheel events. The terminal
// mouse is captured for this (which is why text selection needs
// shift+drag); everything else is ignored.
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.viewport.ScrollUp(wheelScrollLines)
		m.updateScrollState()
	case tea.MouseButtonWheelDown:
		m.viewport.ScrollDown(wheelScrollLines)
		m.updateScrollState()
	}
	return m, nil
}

func (m *Model) appendLines(lines []string) {
	for _, line := range lines {
		m.scrollback.Append(line)
	}
	m.viewport.OnNewLines(len(lines))
	m.updateScrollState()
}

// sendLine hands a submitted input line to the session, dropping (with
// a visible warning) rather than blocking the render loop.
func (m *Model) sendLine(line string) {
	select {
	case m.inputChan <- line:
	default:
		m.scrollback.Append(text.Red("[WARNING] Input dropped - engine lagging"))
	}
}

func (m *Model) isBound(key string) bool {
	return m.boundKeys[key]
}

func (m *Model) sendOutbound(msg ui.UIEvent) {
	if m.outbound == nil {
		return
	}
	select {
	case m.outbound <- msg:
	default:
		// The session is not draining UI events. Dropping is the only
		// safe option here (blocking would deadlock the render loop),
		// but it must never be silent: a lost InputChangedMsg desyncs
		// completion state, a lost PickerSelectMsg strands a picker
		// callback. Make it visible so it can be reported.
		m.scrollback.Append(text.Red("[WARNING] UI event dropped - engine lagging"))
	}
}

func (m *Model) updateScrollState() {
	mode := m.viewport.Mode()
	newLines := m.viewport.NewLineCount()

	modeStr := "live"
	if mode != widget.ModeLive {
		modeStr = "scrolled"
	}
	m.sendOutbound(ui.ScrollStateChangedMsg{Mode: modeStr, NewLines: newLines})
}

// handleScrollKey handles viewport scrolling keys.
// Returns true if the key was handled.
func (m *Model) handleScrollKey(keyType tea.KeyType) bool {
	switch keyType {
	case tea.KeyPgUp:
		m.viewport.PageUp()
	case tea.KeyPgDown:
		m.viewport.PageDown()
	case tea.KeyHome:
		m.viewport.GotoTop()
	case tea.KeyEnd:
		m.viewport.GotoBottom()
	default:
		return false
	}
	m.updateScrollState()
	return true
}
