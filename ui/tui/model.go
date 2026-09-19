package tui

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// defaultRenderInterval bounds screen rendering to about 60 a second.
const defaultRenderInterval = 16 * time.Millisecond

// renderTick ends a throttle window. Bubble Tea calls View after every
// message, and rendering the screen is the one expensive step, so Model
// renders at most once per interval: the first change after an idle period
// renders immediately and starts a window; changes arriving inside it apply
// to state at once and are rendered together when the tick ends it. Ticks
// are scheduled on demand only; at most one renderTick is ever outstanding.
// Bubble Tea owns the separate terminal flush clock.
type renderTick struct{}

// Model is the main Bubble Tea model for the TUI. It routes messages
// between the session and the widgets; input-mode policy lives in the
// inputController; layout planning and canvas rendering are separate.
type Model struct {
	// Layout
	bars   map[string]*widget.Bar // bar namespace
	styles style.Styles

	// Widgets
	output *widget.Output
	input  *widget.Input
	panes  map[string]pane

	// Input-mode state machine (normal / modal picker / inline picker / search)
	inputCtl *inputController

	// Output geometry and focus for the active/committed search result.
	searchView searchViewState

	// Push-based state from Session
	layout     ui.LayoutTree
	layoutPlan layoutPlan

	// Render throttle. Only visible changes mark the screen dirty. A zero
	// renderInterval renders on every View, for deterministic tests.
	renderInterval time.Duration
	throttled      bool
	dirty          bool
	screen         string
	renders        int

	// Renderer state reused between renders: the cell grid and the styled
	// border cell for each junction glyph.
	canvas      uv.ScreenBuffer
	borderCells map[string]*uv.Cell

	// Last scroll state the Session queue accepted; see reportScrollState.
	reportedScroll ui.ScrollStateChangedMsg

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
	output := widget.NewOutput(100000, styles)
	search := widget.NewSearch(output.Scrollback(), styles)
	input := widget.NewInput(styles, search)

	m := &Model{
		output:      output,
		input:       input,
		panes:       map[string]pane{ui.OutputPaneName: output},
		events:      events,
		bars:        make(map[string]*widget.Bar),
		borderCells: make(map[string]*uv.Cell),
		styles:      styles,
		layout:      ui.DefaultLayoutTree(),
		// Session starts from the same value, so an untouched output window
		// reports nothing.
		reportedScroll: ui.ScrollStateChangedMsg{Mode: "live"},
	}
	m.inputCtl = newInputController(input, m.notifySession, m.submit, m.handleScrollKey, m)

	return m
}

// Init implements tea.Model. No standing tick: renderTicks are scheduled on
// demand when something changes.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update applies a message, resolves changed geometry, and renders when due.
// Ordinary output only appends and marks pixels dirty. View returns the last
// rendered screen and never changes session-visible scroll state.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	render, layout := m.dispatch(msg)

	previousOutput := m.layoutPlan.output
	if layout {
		m.applyLayout()
	}
	m.applySearchPosition(previousOutput != m.layoutPlan.output)

	if render {
		m.dirty = true
	}
	return m, m.renderThrottled()
}

// renderThrottled renders a dirty screen unless throttled, then throttles
// until the renderTick it returns. Scroll state is reported with the screen
// it describes, so rune.state matches what the user sees and a flood costs
// Session one report per interval. With a zero interval, View renders and
// every update reports.
func (m *Model) renderThrottled() tea.Cmd {
	if m.renderInterval <= 0 {
		m.reportScrollState()
		return nil
	}
	if m.throttled || !m.dirty {
		return nil
	}
	m.render()
	if !m.reportScrollState() {
		m.dirty = true // retry on the tick
	}
	m.throttled = true
	return tea.Tick(m.renderInterval, func(time.Time) tea.Msg { return renderTick{} })
}

// dispatch applies one message to state. Nothing here renders, reports
// scroll state, or schedules work: Update does that once, afterwards.
// The return values describe changed pixels and changed geometry. Text arriving
// in ordinary panes needs rendering, but does not resize unrelated widgets.
func (m *Model) dispatch(msg tea.Msg) (render, layout bool) {
	switch msg := msg.(type) {
	// System
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.initialized = true
		m.notifySession(ui.WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
	case renderTick:
		// Update renders and re-arms only if the screen went dirty while
		// throttled; otherwise the chain ends - back to zero wakeups.
		m.throttled = false
		return false, false
	case tea.KeyPressMsg:
		m.inputCtl.HandleKey(msg)
	case tea.PasteMsg:
		m.inputCtl.HandlePaste(msg.Content)
	case tea.MouseWheelMsg:
		m.handleMouseWheel(msg)

	// Session config updates
	case ui.UpdateBindsMsg:
		m.input.SetBindings(input.Bindings(msg))
	case ui.UpdateBarsMsg:
		changed := m.syncBars(msg)
		return changed, changed
	case ui.UpdateLayoutMsg:
		m.layout = ui.LayoutTree(msg)
	case ui.UpdateConfigMsg:
		m.inputCtl.SetKeepOnSubmit(msg.KeepInput)
		m.mouseEnabled = msg.Mouse
		m.numpadMode = msg.Numpad

	// Scrollback appends and the prompt overlay. Server lines and local
	// echoes differ only in where Session sends them from.
	case ui.PrintLineMsg:
		m.output.Write(string(msg))
		return true, m.layoutPlan.autoPanes[ui.OutputPaneName]
	case ui.EchoLineMsg:
		m.output.Write(string(msg))
		return true, m.layoutPlan.autoPanes[ui.OutputPaneName]
	case ui.SetPromptMsg:
		changed := m.output.SetPrompt(string(msg))
		return changed, changed && m.layoutPlan.autoPanes[ui.OutputPaneName]
	case ui.CommitPromptMsg:
		changed := m.output.CommitPrompt(string(msg))
		return changed, changed && m.layoutPlan.autoPanes[ui.OutputPaneName]

	// Pane buffer content. Placement and visibility are layout-tree state
	// and arrive as UpdateLayoutMsg instead.
	case ui.PaneCreateMsg:
		m.pane(msg.Name)
		return false, false
	case ui.PaneWriteMsg:
		m.pane(msg.Name).Write(msg.Text)
		return true, m.layoutPlan.autoPanes[msg.Name]
	case ui.PaneReplaceMsg:
		m.dropOutputSearch(msg.Name)
		p := m.pane(msg.Name)
		p.Clear()
		p.Write(msg.Text)
	case ui.PaneClearMsg:
		m.dropOutputSearch(msg.Name)
		if p, ok := m.panes[msg.Name]; ok {
			p.Clear()
		}

	// Input control
	case ui.ShowPickerMsg:
		m.inputCtl.ShowPicker(msg)
	case ui.ShowSearchMsg:
		m.inputCtl.ShowSearch(msg)
	case ui.SetInputMsg:
		m.inputCtl.SetText(string(msg))
	case ui.SetInputSubmissionMsg:
		m.inputCtl.SetSubmission(input.Submission(msg))

	// Input primitives (from Lua)
	case ui.InputSetCursorMsg:
		m.input.SetCursor(int(msg))
		m.notifySession(ui.DraftAppliedMsg{Text: m.input.Value(), Cursor: m.input.Position()})

	// Clipboard (from Lua). OSC 52 asks the terminal emulator to set
	// the system clipboard; it renders nothing, so it bypasses the
	// renderer and goes to the terminal on stderr.
	case ui.SetClipboardMsg:
		osc52.New(string(msg)).WriteTo(os.Stderr) //nolint:errcheck // best-effort: no way to report terminal-side failure
		return false, false

	// Pane scrolling (from Lua). Every named pane follows the same pane
	// contract.
	case ui.PaneScrollUpMsg:
		m.scrollPane(msg.Name, func(pane pane) { pane.ScrollUp(msg.Lines) })
	case ui.PaneScrollDownMsg:
		m.scrollPane(msg.Name, func(pane pane) { pane.ScrollDown(msg.Lines) })
	case ui.PaneScrollToTopMsg:
		m.scrollPane(msg.Name, func(pane pane) { pane.ScrollToTop() })
	case ui.PaneScrollToBottomMsg:
		m.scrollPane(msg.Name, func(pane pane) { pane.ScrollToBottom() })
	default:
		return false, false
	}
	return true, true
}

// syncBars reconciles the bar registry with the latest successful Lua snapshot.
// Panes and built-in widgets have separate owners and namespaces.
func (m *Model) syncBars(content map[string]ui.BarContent) (changed bool) {
	for name := range m.bars {
		if _, exists := content[name]; !exists {
			delete(m.bars, name)
			changed = true
		}
	}

	for name, barContent := range content {
		bar, exists := m.bars[name]
		if !exists {
			bar = widget.NewBar()
			m.bars[name] = bar
			changed = true
		}
		changed = bar.SetContent(barContent) || changed
	}
	return changed
}

// dropOutputSearch abandons an active scrollback search before the output
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

// scrollPane applies one navigation operation to an existing pane.
func (m *Model) scrollPane(name string, scroll func(pane)) {
	pane, ok := m.panes[name]
	if !ok {
		return
	}
	if pane == m.output {
		m.clearCommittedSearchFocus()
	}
	scroll(pane)
}

// wheelScrollLines is how far one mouse-wheel tick scrolls the output
// output window. Matches the common terminal-emulator default.
const wheelScrollLines = 3

// handleMouseWheel moves the search selection while search is open and
// scrolls the output window otherwise.
func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) {
	switch msg.Button {
	case tea.MouseWheelUp:
		if !m.inputCtl.selectOlderSearch() {
			m.clearCommittedSearchFocus()
			m.output.ScrollUp(wheelScrollLines)
		}
	case tea.MouseWheelDown:
		if !m.inputCtl.selectNewerSearch() {
			m.clearCommittedSearchFocus()
			m.output.ScrollDown(wheelScrollLines)
		}
	}
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
		m.output.Write(text.Red(warning))
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

func (m *Model) showWarning(message string) {
	m.output.Write(text.Red("[WARNING] " + message))
}

// reportScrollState posts the output window's scroll state when it differs
// from the last value Session accepted. It is derived state with one reporter:
// nothing else posts it. A full queue is not a dropped event - the value is
// simply still unreported - so it reports false and the caller retries.
func (m *Model) reportScrollState() bool {
	state := ui.ScrollStateChangedMsg{Mode: "live", NewLines: m.output.NewLineCount()}
	if m.output.Mode() != widget.ModeLive {
		state.Mode = "scrolled"
	}
	if state == m.reportedScroll {
		return true
	}
	if !m.tryPost(state) {
		return false
	}
	m.reportedScroll = state
	return true
}

// handleScrollKey handles output window scrolling keys.
// Returns true if the key was handled.
func (m *Model) handleScrollKey(msg tea.KeyPressMsg) bool {
	var move func()
	switch {
	case matchesKey(msg, tea.KeyPgUp, 0):
		move = m.output.PageUp
	case matchesKey(msg, tea.KeyPgDown, 0):
		move = m.output.PageDown
	case matchesKey(msg, tea.KeyHome, tea.ModCtrl):
		move = m.output.ScrollToTop
	case matchesKey(msg, tea.KeyEnd, tea.ModCtrl):
		move = m.output.ScrollToBottom
	default:
		return false
	}
	m.clearCommittedSearchFocus()
	move()
	return true
}
