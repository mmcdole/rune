package tui

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	osc52 "github.com/aymanbagabas/go-osc52/v2"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// defaultFrameInterval bounds screen composition to about 60 frames a second.
const defaultFrameInterval = 16 * time.Millisecond

// frameMsg closes the current frame. Bubble Tea calls View after every
// message, and composing the screen is the one expensive step, so Model
// composes at most once per frame: the first change after an idle period
// composes immediately and opens a frame; changes arriving inside it apply
// to state at once and are composed together when the frame closes. Frames
// are scheduled on demand only - an idle client has no standing timer and
// zero wakeups - and at most one frameMsg is ever outstanding.
type frameMsg struct{}

// Model is the main Bubble Tea model for the TUI. It routes messages
// between the session and the widgets; input-mode policy lives in the
// inputController; layout planning and canvas rendering are separate.
type Model struct {
	// Layout
	bars   map[string]*widget.Bar // bar resource namespace
	styles style.Styles

	// Widgets
	output *outputController
	input  *widget.Input
	panes  *paneRegistry

	// Input-mode state machine (normal / modal picker / inline picker / search)
	inputCtl *inputController

	// Viewport geometry and focus for the active/committed search result.
	searchView searchViewState

	// Push-based state from Session
	layout     ui.LayoutTree
	layoutPlan layoutPlan

	// Frame clock. Every message but frameMsg marks the screen stale; see
	// frameMsg for when a stale screen is composed. A zero frameInterval
	// composes on every View instead.
	frameInterval   time.Duration
	frameOpen       bool
	stale           bool
	renderedContent string
	compositions    int

	// A server line inside an open frame reports scroll state when the frame
	// closes, so a flood reports once per frame rather than once per line.
	scrollReportDue bool

	// State
	width        int
	height       int
	events       chan<- ui.UIEvent
	mouseEnabled bool
	numpadMode   bool
	initialized  bool
}

// NewModel creates a new TUI model.
func NewModel(events chan<- ui.UIEvent) *Model {
	styles := style.DefaultStyles()
	output := newOutputController(styles)
	search := widget.NewSearch(output.buffer, styles)
	input := widget.NewInput(styles, search)
	panes := newPaneRegistry(output)

	m := &Model{
		output: output,
		input:  input,
		panes:  panes,
		events: events,
		bars:   make(map[string]*widget.Bar),
		styles: styles,
		layout: ui.DefaultLayoutTree(),
	}
	m.inputCtl = newInputController(input, m.notifySession, m.submit, m.handleScrollKey, m)

	return m
}

// Init implements tea.Model. No standing tick: frames are scheduled on
// demand when something changes.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model: apply the message, finalize geometry once and
// the navigation that depends on it, then let the frame clock decide whether
// to compose. View only returns the composed screen; it never changes
// session-visible scroll state.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.dispatch(msg)

	previousOutput := m.layoutPlan.output
	changed := m.applyLayout()
	if m.applySearchPosition(previousOutput != m.layoutPlan.output) || changed {
		m.updateScrollState()
	}

	if _, closing := msg.(frameMsg); !closing {
		m.stale = true
	}
	return m, tea.Batch(cmd, m.advanceFrame())
}

// advanceFrame composes a stale screen unless a frame is open, and returns
// the command that closes the frame it opened.
func (m *Model) advanceFrame() tea.Cmd {
	if m.frameInterval <= 0 || m.frameOpen || !m.stale {
		return nil
	}
	m.compose()
	m.frameOpen = true
	return tea.Tick(m.frameInterval, func(time.Time) tea.Msg { return frameMsg{} })
}

func (m *Model) dispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// System
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case frameMsg:
		return m.handleFrame()
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
	case ui.PaneCreateMsg, ui.PaneWriteMsg, ui.PaneReplaceMsg, ui.PaneClearMsg:
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
		m.notifySession(ui.DraftAppliedMsg{Text: m.input.Value(), Cursor: m.input.Position()})
		return m, nil

	// Clipboard (from Lua). OSC 52 asks the terminal emulator to set
	// the system clipboard; it renders nothing, so it bypasses the
	// renderer and goes to the terminal on stderr.
	case ui.SetClipboardMsg:
		osc52.New(string(msg)).WriteTo(os.Stderr) //nolint:errcheck // best-effort: no way to report terminal-side failure
		return m, nil

	// Pane scrolling (from Lua). Every named surface follows the same pane
	// contract. Output additionally reports its scroll state to Session.
	case ui.PaneScrollUpMsg:
		m.scrollPane(msg.Name, func(pane paneResource) { pane.ScrollUp(msg.Lines) })
		return m, nil
	case ui.PaneScrollDownMsg:
		m.scrollPane(msg.Name, func(pane paneResource) { pane.ScrollDown(msg.Lines) })
		return m, nil
	case ui.PaneScrollToTopMsg:
		m.scrollPane(msg.Name, func(pane paneResource) { pane.ScrollToTop() })
		return m, nil
	case ui.PaneScrollToBottomMsg:
		m.scrollPane(msg.Name, func(pane paneResource) { pane.ScrollToBottom() })
		return m, nil
	}

	return m, nil
}

func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.initialized = true
	m.notifySession(ui.WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
	return m, nil
}

// handleFrame closes the current frame. Update then composes and re-arms only
// if the screen went stale inside it; otherwise the chain ends - back to zero
// wakeups.
func (m *Model) handleFrame() (tea.Model, tea.Cmd) {
	m.frameOpen = false
	if m.scrollReportDue {
		m.scrollReportDue = false
		m.updateScrollState()
	}
	return m, nil
}

func (m *Model) handleConfigUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.UpdateBindsMsg:
		m.input.SetBindings(input.Bindings(msg))
	case ui.UpdateBarsMsg:
		m.syncBars(msg)
	case ui.UpdateLayoutMsg:
		m.layout = ui.LayoutTree(msg)
	case ui.UpdateConfigMsg:
		m.inputCtl.SetKeepOnSubmit(msg.KeepInput)
		m.mouseEnabled = msg.Mouse
		m.numpadMode = msg.Numpad
	}
	return m, nil
}

// syncBars reconciles the bar registry with the latest successful Lua snapshot.
// Panes and built-in widgets have separate owners and namespaces.
func (m *Model) syncBars(content map[string]ui.BarContent) {
	for name := range m.bars {
		if _, exists := content[name]; !exists {
			delete(m.bars, name)
		}
	}

	for name, barContent := range content {
		bar, exists := m.bars[name]
		if !exists {
			bar = widget.NewBar()
			m.bars[name] = bar
		}
		bar.SetContent(barContent)
	}
}

func (m *Model) handleDisplayOutput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.PrintLineMsg:
		m.output.Write(string(msg))
		if m.frameOpen {
			m.scrollReportDue = true
			return m, nil
		}
		m.updateScrollState()
	case ui.EchoLineMsg:
		m.output.Write(string(msg))
		m.updateScrollState()
	case ui.SetPromptMsg:
		m.output.setPrompt(string(msg))
	case ui.CommitPromptMsg:
		m.output.commitPrompt(string(msg))
		m.updateScrollState()
	}
	return m, nil
}

// handlePaneMsg applies buffer content operations. Placement and visibility
// are layout-tree state and arrive as UpdateLayoutMsg instead.
func (m *Model) handlePaneMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	contentChanged := false
	var affected paneResource
	switch msg := msg.(type) {
	case ui.PaneCreateMsg:
		affected = m.panes.Create(msg.Name)
	case ui.PaneWriteMsg:
		affected = m.panes.Write(msg.Name, msg.Text)
		contentChanged = true
	case ui.PaneReplaceMsg:
		m.dropOutputSearch(msg.Name)
		affected = m.panes.Replace(msg.Name, msg.Text)
		contentChanged = true
	case ui.PaneClearMsg:
		m.dropOutputSearch(msg.Name)
		affected, _ = m.panes.Clear(msg.Name)
		contentChanged = true
	}
	if affected == m.output && contentChanged {
		m.updateScrollState()
	}
	return m, nil
}

// dropOutputSearch abandons an active transcript search before the output
// buffer it anchors to is emptied.
func (m *Model) dropOutputSearch(name string) {
	if name != ui.OutputPaneName {
		return
	}
	if m.input.SearchActive() {
		m.inputCtl.closeSearch(false)
	}
	m.searchView = searchViewState{}
}

// scrollPane applies one navigation operation to an existing pane. Output's
// extra search and session-state effects remain private to the controller.
func (m *Model) scrollPane(name string, scroll func(paneResource)) {
	pane, ok := m.panes.Lookup(name)
	if !ok {
		return
	}
	if pane == m.output {
		m.navigateOutputPane(func() { scroll(pane) })
		return
	}
	scroll(pane)
}

// wheelScrollLines is how far one mouse-wheel tick scrolls the output
// viewport. Matches the common terminal-emulator default.
const wheelScrollLines = 3

// handleMouse scrolls the output viewport on wheel events when the terminal
// reports them; everything else is ignored.
func (m *Model) handleMouse(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		if m.inputCtl.selectOlderSearch() {
			return m, nil
		}
		m.navigateOutputPane(func() {
			m.output.viewport.ScrollUp(wheelScrollLines)
		})
	case tea.MouseWheelDown:
		if m.inputCtl.selectNewerSearch() {
			return m, nil
		}
		m.navigateOutputPane(func() {
			m.output.viewport.ScrollDown(wheelScrollLines)
		})
	}
	return m, nil
}

// appendMessage shapes text into rows and appends them.
func (m *Model) appendMessage(text string) {
	m.output.Write(text)
	m.updateScrollState()
}

// submit offers a submission and its following draft to the session as one
// transition. It rejects invalid command text or a busy engine with a
// visible warning rather than blocking the render loop; false tells the
// controller to retain the current local draft.
func (m *Model) submit(msg ui.InputSubmittedMsg) bool {
	if msg.Submission.Mode == input.ModeCommand && !input.ValidCommandText(msg.Submission.Text) {
		warning := "[WARNING] Command not run - invalid text or terminal controls."
		if key := m.input.Bindings().Hint("toggle_mode"); key != "" {
			warning += " Use " + key + " for verbatim."
		}
		m.appendMessage(text.Red(warning))
		return false
	}
	if m.tryPost(msg) {
		return true
	}
	m.showWarning("Input not sent - engine lagging")
	return false
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
	m.output.Write(text.Red("[WARNING] " + message))
}

func (m *Model) updateScrollState() {
	mode := m.output.viewport.Mode()
	newLines := m.output.viewport.NewLineCount()

	modeStr := "live"
	if mode != widget.ModeLive {
		modeStr = "scrolled"
	}
	m.notifySession(ui.ScrollStateChangedMsg{Mode: modeStr, NewLines: newLines})
}

// navigateOutputPane is the single path for deliberate user/script
// navigation of the output surface. Search previews position the
// viewport directly so their committed marker remains intact.
func (m *Model) navigateOutputPane(move func()) {
	m.clearCommittedSearchFocus()
	move()
	m.updateScrollState()
}

// handleScrollKey handles viewport scrolling keys.
// Returns true if the key was handled.
func (m *Model) handleScrollKey(msg tea.KeyPressMsg) bool {
	switch {
	case matchesKey(msg, tea.KeyPgUp, 0):
		m.navigateOutputPane(m.output.viewport.PageUp)
	case matchesKey(msg, tea.KeyPgDown, 0):
		m.navigateOutputPane(m.output.viewport.PageDown)
	case matchesKey(msg, tea.KeyHome, tea.ModCtrl):
		m.navigateOutputPane(m.output.viewport.GotoTop)
	case matchesKey(msg, tea.KeyEnd, tea.ModCtrl):
		m.navigateOutputPane(m.output.viewport.GotoBottom)
	default:
		return false
	}
	return true
}
