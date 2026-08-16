package tui

import (
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	osc52 "github.com/aymanbagabas/go-osc52/v2"

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
	styles  style.Styles

	// Widgets
	scrollback *widget.ScrollbackBuffer
	viewport   *widget.Viewport
	input      *widget.Input
	panes      *widget.PaneManager

	// Input-mode state machine (normal / modal picker / inline picker / search)
	inputCtl *inputController

	// Viewport geometry and focus for the active/committed search result.
	searchView searchViewState

	// Push-based state from Session
	boundKeys  map[string]bool
	barContent map[string]ui.BarContent
	luaLayout  struct {
		Top    []ui.LayoutEntry
		Bottom []ui.LayoutEntry
	}

	// State
	promptText   string
	width        int
	height       int
	events       chan<- ui.UIEvent
	mouseEnabled bool
	initialized  bool
	pendingRows  []string
	// flushScheduled is true while a batch-window tick is outstanding.
	// At most one tick is ever in flight: it is armed only on the
	// idle->hot transition and re-armed only from handleTick while
	// output is still flowing.
	flushScheduled bool
}

// NewModel creates a new TUI model.
func NewModel(events chan<- ui.UIEvent) *Model {
	styles := style.DefaultStyles()
	scrollback := widget.NewScrollbackBuffer(100000)
	viewport := widget.NewViewport(scrollback, styles)
	search := widget.NewSearch(scrollback, styles)
	input := widget.NewInput(styles, search)
	panes := widget.NewPaneManager()

	m := &Model{
		scrollback: scrollback,
		viewport:   viewport,
		input:      input,
		panes:      panes,
		events:     events,
		widgets:    make(map[string]widget.Widget),
		styles:     styles,
	}
	m.inputCtl = newInputController(input, m.notifySession, m.submit, m.isBound, m.handleScrollKey, m)

	// Register static widgets
	m.widgets["input"] = input
	m.widgets["separator"] = widget.NewSeparator()

	return m
}

// Init implements tea.Model. No standing tick: batch-window ticks are
// scheduled on demand when server output arrives.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// System
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tickMsg:
		return m.handleTick()
	case tea.KeyPressMsg:
		m.inputCtl.HandleKey(msg)
		return m, nil
	case tea.PasteMsg:
		m.inputCtl.HandlePaste(msg.Content)
		return m, nil
	case tea.MouseWheelMsg:
		return m.handleMouse(msg)

	// Session config updates
	case ui.UpdateBindsMsg, ui.UpdateBarsMsg, ui.UpdateLayoutMsg, ui.UpdateConfigMsg:
		return m.handleConfigUpdate(msg)

	// Scrollback appends and the prompt overlay
	case ui.PrintLineMsg, ui.EchoLineMsg, ui.SetPromptMsg, ui.CommitPromptMsg:
		return m.handleDisplayOutput(msg)

	// Pane operations
	case ui.PaneCreateMsg, ui.PaneWriteMsg, ui.PaneToggleMsg, ui.PaneSetVisibleMsg, ui.PaneClearMsg:
		return m.handlePaneMsg(msg)

	// Input control
	case ui.ShowPickerMsg:
		m.inputCtl.ShowPicker(msg)
		return m, nil
	case ui.ShowSearchMsg:
		m.inputCtl.ShowSearch(msg)
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

	// Clipboard (from Lua). OSC 52 asks the terminal emulator to set
	// the system clipboard; it renders nothing, so it bypasses the
	// renderer and goes to the terminal on stderr.
	case ui.SetClipboardMsg:
		osc52.New(string(msg)).WriteTo(os.Stderr) //nolint:errcheck // best-effort: no way to report terminal-side failure
		return m, nil

	// Pane scrolling (from Lua). "main" is the output viewport; any
	// other name scrolls that pane's own buffer. Unknown panes are
	// ignored rather than auto-created.
	case ui.PaneScrollUpMsg:
		if msg.Name == "main" {
			m.navigateMainViewport(func() {
				m.viewport.ScrollUp(msg.Lines)
			})
		} else if pane, ok := m.panes.Lookup(msg.Name); ok {
			pane.ScrollUp(msg.Lines)
		}
		return m, nil
	case ui.PaneScrollDownMsg:
		if msg.Name == "main" {
			m.navigateMainViewport(func() {
				m.viewport.ScrollDown(msg.Lines)
			})
		} else if pane, ok := m.panes.Lookup(msg.Name); ok {
			pane.ScrollDown(msg.Lines)
		}
		return m, nil
	case ui.PaneScrollToTopMsg:
		if msg.Name == "main" {
			m.navigateMainViewport(m.viewport.GotoTop)
		} else if pane, ok := m.panes.Lookup(msg.Name); ok {
			pane.ScrollToTop()
		}
		return m, nil
	case ui.PaneScrollToBottomMsg:
		if msg.Name == "main" {
			m.navigateMainViewport(m.viewport.GotoBottom)
		} else if pane, ok := m.panes.Lookup(msg.Name); ok {
			pane.ScrollToBottom()
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.initialized = true
	m.syncViewportSize()
	scrollStateChanged := m.recenterSearchFocus()
	m.notifySession(ui.WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
	if scrollStateChanged {
		m.updateScrollState()
	}
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
	layoutChanged := false
	switch msg := msg.(type) {
	case ui.UpdateBindsMsg:
		m.boundKeys = msg
	case ui.UpdateBarsMsg:
		m.syncBars(msg)
		layoutChanged = true
	case ui.UpdateLayoutMsg:
		m.luaLayout.Top = msg.Top
		m.luaLayout.Bottom = msg.Bottom
		layoutChanged = true
	case ui.UpdateConfigMsg:
		m.inputCtl.SetKeepOnSubmit(msg.KeepInput)
		m.mouseEnabled = msg.Mouse
	}
	if layoutChanged {
		m.syncViewportSize()
		if m.recenterSearchFocus() {
			m.updateScrollState()
		}
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

func (m *Model) handleDisplayOutput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.PrintLineMsg:
		rows := splitRows(string(msg), m.width)
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
	case ui.SetPromptMsg:
		text := util.ExpandTabs(string(msg))
		if text != m.promptText {
			m.viewport.SetPrompt(text)
			m.promptText = text
		}
	case ui.CommitPromptMsg:
		m.flushPending()
		if text := util.ExpandTabs(string(msg)); text != "" {
			m.appendMessage(text)
		}
		m.viewport.SetPrompt("")
		m.promptText = ""
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

// handleMouse scrolls the main viewport on wheel events when the terminal
// reports them; everything else is ignored.
func (m *Model) handleMouse(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		if m.inputCtl.selectOlderSearch() {
			return m, nil
		}
		m.navigateMainViewport(func() {
			m.viewport.ScrollUp(wheelScrollLines)
		})
	case tea.MouseWheelDown:
		if m.inputCtl.selectNewerSearch() {
			return m, nil
		}
		m.navigateMainViewport(func() {
			m.viewport.ScrollDown(wheelScrollLines)
		})
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

// submit offers a submission and its following draft to the session as one
// transition. It rejects oversized verbatim drafts or a busy engine with a
// visible warning rather than blocking the render loop; false tells the
// controller to retain the current local draft.
func (m *Model) submit(msg ui.InputSubmittedMsg) bool {
	if msg.Submission.Mode == input.ModeVerbatim {
		lineCount := len(msg.Submission.PhysicalLines())
		if len(msg.Submission.Text) > maxVerbatimBytes || lineCount > maxVerbatimLines {
			// A size rejection is local validation, not queue pressure, so
			// the normal reporting append keeps scroll state in sync.
			m.appendMessage(text.Red("[WARNING] Verbatim input not sent - limit is 1000 lines or 256 KiB"))
			return false
		}
	}
	if m.tryPost(msg) {
		return true
	}
	m.showWarning("Input not sent - engine lagging")
	return false
}

const (
	maxVerbatimBytes = 256 * 1024
	maxVerbatimLines = 1000
)

func (m *Model) isBound(key string) bool {
	return m.boundKeys[key]
}

func (m *Model) tryPost(event ui.UIEvent) bool {
	select {
	case m.events <- event:
		return true
	default:
		return false
	}
}

func (m *Model) notifySession(event ui.UIEvent) {
	if m.tryPost(event) {
		return
	}
	// Blocking would deadlock the render loop, but a lost event must be
	// visible: it can desync input state or strand a picker callback.
	m.showWarning("UI event dropped - engine lagging")
}

// showWarning appends locally without reporting another scroll-state event:
// this path is reached only when the Session event queue is already full.
func (m *Model) showWarning(message string) {
	rows := splitRows(text.Red("[WARNING] "+message), m.width)
	for _, row := range rows {
		m.scrollback.Append(row)
	}
	m.viewport.OnNewRows(len(rows))
}

func (m *Model) updateScrollState() {
	mode := m.viewport.Mode()
	newLines := m.viewport.NewLineCount()

	modeStr := "live"
	if mode != widget.ModeLive {
		modeStr = "scrolled"
	}
	m.notifySession(ui.ScrollStateChangedMsg{Mode: modeStr, NewLines: newLines})
}

// navigateMainViewport is the single path for deliberate user/script
// navigation of the main output surface. Search previews position the
// viewport directly so their committed marker remains intact.
func (m *Model) navigateMainViewport(move func()) {
	m.clearCommittedSearchFocus()
	move()
	m.updateScrollState()
}

// handleScrollKey handles viewport scrolling keys.
// Returns true if the key was handled.
func (m *Model) handleScrollKey(msg tea.KeyPressMsg) bool {
	switch {
	case matchesKey(msg, tea.KeyPgUp, 0):
		m.navigateMainViewport(m.viewport.PageUp)
	case matchesKey(msg, tea.KeyPgDown, 0):
		m.navigateMainViewport(m.viewport.PageDown)
	case matchesKey(msg, tea.KeyHome, tea.ModCtrl):
		m.navigateMainViewport(m.viewport.GotoTop)
	case matchesKey(msg, tea.KeyEnd, tea.ModCtrl):
		m.navigateMainViewport(m.viewport.GotoBottom)
	default:
		return false
	}
	return true
}
