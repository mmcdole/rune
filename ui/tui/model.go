package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// tickMsg closes a 16ms output batch window: the first server line
// after an idle period renders immediately and opens the window; lines
// arriving inside it are batched to prevent excessive renders on fast
// MUD output. Ticks are scheduled on demand only - an idle client has
// no standing timer and zero wakeups.
type tickMsg time.Time

// doTick returns a command that closes the batch window after 16ms.
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
	lastPrompt  string
	width       int
	height      int
	inputChan   chan<- input.Submission
	outbound    chan<- ui.UIEvent
	initialized bool
	pendingRows []string
	// flushScheduled is true while a batch-window tick is outstanding.
	// At most one tick is ever in flight: it is armed only on the
	// idle->hot transition and re-armed only from handleTick while
	// output is still flowing.
	flushScheduled bool
}

// NewModel creates a new TUI model.
func NewModel(inputChan chan<- input.Submission, outbound chan<- ui.UIEvent) *Model {
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

// Init implements tea.Model. No standing tick: batch-window ticks are
// scheduled on demand when server output arrives.
func (m *Model) Init() tea.Cmd {
	return tea.EnterAltScreen
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
	case ui.PaneCreateMsg, ui.PaneWriteMsg, ui.PaneToggleMsg, ui.PaneSetVisibleMsg, ui.PaneClearMsg:
		return m.handlePaneMsg(msg)

	// Input control
	case ui.ShowPickerMsg:
		m.inputCtl.ShowPicker(msg)
		return m, nil
	case ui.SetInputMsg:
		m.inputCtl.SetText(string(msg))
		return m, nil
	case ui.SetInputSubmissionMsg:
		m.inputCtl.SetSubmission(input.Submission(msg))
		return m, nil

	// Input primitives (from Lua)
	case ui.InputSetCursorMsg:
		m.input.SetCursor(int(msg))
		return m, nil

	// Pane scrolling (from Lua). "main" is the output viewport; any
	// other name scrolls that pane's own buffer. Unknown panes are
	// ignored rather than auto-created.
	case ui.PaneScrollUpMsg:
		if msg.Name == "main" {
			m.viewport.ScrollUp(msg.Lines)
			m.updateScrollState()
		} else if m.panes.Exists(msg.Name) {
			m.panes.Get(msg.Name).ScrollUp(msg.Lines)
		}
		return m, nil
	case ui.PaneScrollDownMsg:
		if msg.Name == "main" {
			m.viewport.ScrollDown(msg.Lines)
			m.updateScrollState()
		} else if m.panes.Exists(msg.Name) {
			m.panes.Get(msg.Name).ScrollDown(msg.Lines)
		}
		return m, nil
	case ui.PaneScrollToTopMsg:
		if msg.Name == "main" {
			m.viewport.GotoTop()
			m.updateScrollState()
		} else if m.panes.Exists(msg.Name) {
			m.panes.Get(msg.Name).ScrollToTop()
		}
		return m, nil
	case ui.PaneScrollToBottomMsg:
		if msg.Name == "main" {
			m.viewport.GotoBottom()
			m.updateScrollState()
		} else if m.panes.Exists(msg.Name) {
			m.panes.Get(msg.Name).ScrollToBottom()
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

// handleTick closes the current batch window: flushes any lines that
// arrived inside it and re-arms the window only while output is still
// flowing. A tick that finds nothing pending (output went quiet, or an
// echo already flushed eagerly) ends the chain - back to zero wakeups.
func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	m.flushScheduled = false
	if len(m.pendingRows) == 0 {
		return m, nil
	}
	m.flushPending()
	m.flushScheduled = true
	return m, doTick()
}

// flushPending appends all batched server rows to the scrollback.
func (m *Model) flushPending() {
	if len(m.pendingRows) == 0 {
		return
	}
	m.appendRows(m.pendingRows...)
	m.pendingRows = nil
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
		rows := splitRows(util.FilterClearSequences(string(msg)), m.width)
		if m.flushScheduled {
			// Inside a batch window: coalesce with the burst.
			m.pendingRows = append(m.pendingRows, rows...)
			return m, nil
		}
		// Idle: render this line now and open a batch window so a
		// following burst coalesces instead of rendering line-by-line.
		m.appendRows(rows...)
		m.flushScheduled = true
		return m, doTick()
	case ui.EchoLineMsg:
		// Flush batched server lines first so the echo cannot render
		// ahead of output that arrived before it.
		m.flushPending()
		m.appendMessage(string(msg))
	case ui.PromptMsg:
		text := util.ExpandTabs(string(msg))
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
	case ui.PaneSetVisibleMsg:
		m.panes.SetVisible(msg.Name, msg.Visible)
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

// splitRows shapes a message into physical scrollback rows: one row
// per line break, tabs expanded per row so columns restart on every
// row, rows wider than the terminal word-wrapped. Rows are final at
// append time; a resize does not rewrap old output.
func splitRows(msg string, width int) []string {
	if !strings.ContainsAny(msg, "\r\n") {
		return util.WrapLine(util.ExpandTabs(msg), width)
	}
	var rows []string
	for _, line := range util.SplitLines(msg) {
		rows = append(rows, util.WrapLine(util.ExpandTabs(line), width)...)
	}
	return rows
}

func (m *Model) appendRows(rows ...string) {
	for _, row := range rows {
		m.scrollback.Append(row)
	}
	m.viewport.OnNewRows(len(rows))
	m.updateScrollState()
}

// appendMessage shapes text into rows and appends them.
func (m *Model) appendMessage(text string) {
	m.appendRows(splitRows(text, m.width)...)
}

// sendLine offers a submitted input snapshot to the session. It rejects
// oversized verbatim drafts or a busy engine with a visible warning rather
// than blocking the render loop; false tells the controller to retain them.
func (m *Model) sendLine(submission input.Submission) bool {
	if submission.Mode == input.ModeVerbatim {
		lineCount := 1 + strings.Count(submission.Text, "\n")
		if len(submission.Text) > maxVerbatimBytes || lineCount > maxVerbatimLines {
			m.appendMessage(text.Red("[WARNING] Verbatim input not sent - limit is 1000 lines or 256 KiB"))
			return false
		}
	}
	select {
	case m.inputChan <- submission:
		return true
	default:
		m.appendMessage(text.Red("[WARNING] Input not sent - engine lagging"))
		return false
	}
}

const (
	maxVerbatimBytes = 256 * 1024
	maxVerbatimLines = 1000
)

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
