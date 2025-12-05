package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
	"github.com/drake/rune/ui/tui/widget"
)

// tickMsg is used for periodic updates (line batching, clock refresh).
type tickMsg time.Time

// doTick returns a command that sends a tickMsg after the given duration.
func doTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// InputMode represents the current input handling mode.
type InputMode int

const (
	ModeNormal       InputMode = iota // Standard text input
	ModePickerModal                   // Modal picker traps all keys
	ModePickerInline                  // Inline picker filters based on input
)

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	// Layout
	widgets map[string]widget.Widget // all named widgets: input, separator, bars

	// Widgets
	scrollback *widget.ScrollbackBuffer
	viewport   *widget.Viewport
	input      *widget.Input
	panes      *widget.PaneManager
	styles     style.Styles

	// Input mode
	inputMode InputMode

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
	outbound     chan<- any
	quitting     bool
	initialized  bool
	pendingLines []string

	// Cached layout state (computed in Update, used in View)
	viewportHeight int
}

// NewModel creates a new TUI model.
func NewModel(inputChan chan<- string, outbound chan<- any) Model {
	styles := style.DefaultStyles()
	scrollback := widget.NewScrollbackBuffer(100000)
	viewport := widget.NewViewport(scrollback)
	input := widget.NewInput(styles)
	panes := widget.NewPaneManager(styles)

	m := Model{
		scrollback: scrollback,
		viewport:   viewport,
		input:      input,
		panes:      panes,
		styles:     styles,
		inputChan:  inputChan,
		outbound:   outbound,
		widgets:    make(map[string]widget.Widget),
	}

	// Register static widgets
	m.widgets["input"] = input
	m.widgets["separator"] = widget.NewSeparator()

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		doTick(),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// System
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tickMsg:
		return m.handleTick()
	case tea.KeyMsg:
		return m.handleKey(msg)

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
		return m.handleShowPicker(msg)
	case ui.SetInputMsg:
		m.input.SetValue(string(msg))
		m.input.CursorEnd()
		return m, nil
	}

	return m, nil
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.initialized = true
	m.recalculateLayout()
	m.sendOutbound(ui.WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
	return m, nil
}

func (m Model) handleTick() (tea.Model, tea.Cmd) {
	if len(m.pendingLines) > 0 {
		m.appendLines(m.pendingLines)
		m.pendingLines = nil
	}
	return m, doTick()
}

func (m Model) handleConfigUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.UpdateBindsMsg:
		m.boundKeys = msg
	case ui.UpdateBarsMsg:
		m.syncBars(msg)
		m.recalculateLayout()
	case ui.UpdateLayoutMsg:
		m.luaLayout.Top = msg.Top
		m.luaLayout.Bottom = msg.Bottom
		m.recalculateLayout()
	}
	return m, nil
}

// syncBars updates the widgets map to match the current bar content.
// Creates new Bar instances for new names, removes stale ones, updates existing ones.
func (m *Model) syncBars(content map[string]ui.BarContent) {
	// Remove bars that no longer exist in content
	for name := range m.barContent {
		if _, exists := content[name]; !exists {
			delete(m.widgets, name)
		}
	}

	// Add or update bars
	for name, barContent := range content {
		bar, exists := m.widgets[name]
		if !exists {
			bar = widget.NewBar(name)
			m.widgets[name] = bar
		}
		bar.(*widget.Bar).SetContent(barContent)
	}

	m.barContent = content
}

func (m Model) handleServerOutput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.PrintLineMsg:
		cleanLine := util.FilterClearSequences(string(msg))
		m.pendingLines = append(m.pendingLines, cleanLine)
		m.input.AddToWordCache(cleanLine)
	case ui.EchoLineMsg:
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

func (m Model) handlePaneMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.PaneCreateMsg:
		m.panes.Create(msg.Name)
		m.recalculateLayout()
	case ui.PaneWriteMsg:
		m.panes.Write(msg.Name, msg.Text)
	case ui.PaneToggleMsg:
		m.panes.Toggle(msg.Name)
		m.recalculateLayout()
	case ui.PaneClearMsg:
		m.panes.Clear(msg.Name)
	}
	return m, nil
}

func (m Model) handleShowPicker(msg ui.ShowPickerMsg) (tea.Model, tea.Cmd) {
	if msg.Inline {
		m.inputMode = ModePickerInline
	} else {
		m.inputMode = ModePickerModal
	}
	m.input.ShowPicker(msg.Items, msg.Title, msg.CallbackID, msg.Inline)
	m.recalculateLayout()
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.input.Value() == "" && m.inputMode == ModeNormal {
			m.quitting = true
			return m, tea.Quit
		}
		if m.inputMode != ModeNormal {
			m.inputMode = ModeNormal
			cbID := m.input.PickerCallbackID()
			m.input.HidePicker()
			m.recalculateLayout()
			m.sendOutbound(ui.PickerSelectMsg{CallbackID: cbID, Accepted: false})
			return m, nil
		}
		m.input.Reset()
		return m, nil

	case tea.KeyEsc:
		if m.inputMode != ModeNormal {
			m.inputMode = ModeNormal
			cbID := m.input.PickerCallbackID()
			m.input.HidePicker()
			m.recalculateLayout()
			m.sendOutbound(ui.PickerSelectMsg{CallbackID: cbID, Accepted: false})
			return m, nil
		}
		m.input.Reset()
		return m, nil
	}

	switch m.inputMode {
	case ModePickerModal:
		return m.handlePickerKey(msg)
	case ModePickerInline:
		return m.handleInlinePickerKey(msg)
	default:
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := keyToString(msg)
	if keyStr != "" && m.boundKeys[keyStr] {
		isPrintable := msg.Type == tea.KeyRunes
		if !isPrintable || m.input.Value() == "" {
			m.sendOutbound(ui.ExecuteBindMsg(keyStr))
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyEnter:
		text := m.input.Value()
		if text != "" {
			m.input.AddInputToWordCache(text)
		}
		select {
		case m.inputChan <- text:
		default:
			m.scrollback.Append("\033[31m[WARNING] Input dropped - engine lagging\033[0m")
		}
		m.input.Reset()
		return m, nil

	case tea.KeyCtrlU:
		m.input.SetValue("")
		return m, nil

	case tea.KeyCtrlW:
		m.input.DeleteWord()
		return m, nil

	case tea.KeyPgUp:
		m.viewport.PageUp()
		m.updateScrollState()
		return m, nil

	case tea.KeyPgDown:
		m.viewport.PageDown()
		m.updateScrollState()
		return m, nil

	case tea.KeyEnd:
		m.viewport.GotoBottom()
		m.updateScrollState()
		return m, nil

	case tea.KeyHome:
		m.viewport.GotoTop()
		m.updateScrollState()
		return m, nil
	}

	// Forward to input
	oldValue := m.input.Value()
	m.input.UpdateTextInput(msg)

	if newValue := m.input.Value(); newValue != oldValue {
		m.sendOutbound(ui.InputChangedMsg(newValue))
	}

	m.input.UpdateSuggestions()
	return m, nil
}

func (m Model) handleInlinePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := keyToString(msg)
	if keyStr != "" && m.boundKeys[keyStr] && keyStr != "up" && keyStr != "down" {
		m.sendOutbound(ui.ExecuteBindMsg(keyStr))
		return m, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		m.input.PickerSelectUp()
		return m, nil

	case tea.KeyDown:
		m.input.PickerSelectDown()
		return m, nil

	case tea.KeyTab:
		if item, ok := m.input.PickerSelected(); ok {
			m.input.SetValue(item.GetValue() + " ")
			m.input.CursorEnd()
			m.sendOutbound(ui.PickerSelectMsg{
				CallbackID: m.input.PickerCallbackID(),
				Value:      item.GetValue(),
				Accepted:   true,
			})
		}
		m.inputMode = ModeNormal
		m.input.HidePicker()
		m.recalculateLayout()
		return m, nil

	case tea.KeyEnter:
		if item, ok := m.input.PickerSelected(); ok {
			m.sendOutbound(ui.PickerSelectMsg{
				CallbackID: m.input.PickerCallbackID(),
				Value:      item.GetValue(),
				Accepted:   true,
			})
		}
		m.inputMode = ModeNormal
		m.input.HidePicker()
		m.recalculateLayout()
		return m.submitInput()

	case tea.KeyCtrlU:
		m.input.SetValue("")
		m.closeInlinePicker()
		return m, nil

	case tea.KeyCtrlW:
		m.input.DeleteWord()
		m.input.UpdatePickerFilter()
		return m, nil

	case tea.KeyPgUp:
		m.viewport.PageUp()
		m.updateScrollState()
		return m, nil

	case tea.KeyPgDown:
		m.viewport.PageDown()
		m.updateScrollState()
		return m, nil

	case tea.KeyEnd:
		m.viewport.GotoBottom()
		m.updateScrollState()
		return m, nil

	case tea.KeyHome:
		m.viewport.GotoTop()
		m.updateScrollState()
		return m, nil
	}

	oldValue := m.input.Value()
	m.input.UpdateTextInput(msg)

	if newValue := m.input.Value(); newValue != oldValue {
		m.sendOutbound(ui.InputChangedMsg(newValue))
		m.input.UpdatePickerFilter()
	}

	m.input.UpdateSuggestions()
	return m, nil
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := m.input.Value()
	if text != "" {
		m.input.AddInputToWordCache(text)
	}
	select {
	case m.inputChan <- text:
	default:
		m.scrollback.Append("\033[31m[WARNING] Input dropped - engine lagging\033[0m")
	}
	m.input.Reset()
	return m, nil
}

func (m *Model) closeInlinePicker() {
	m.inputMode = ModeNormal
	m.sendOutbound(ui.PickerSelectMsg{CallbackID: m.input.PickerCallbackID(), Accepted: false})
	m.input.HidePicker()
	m.recalculateLayout()
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.input.PickerSelectUp()
		return m, nil

	case tea.KeyDown:
		m.input.PickerSelectDown()
		return m, nil

	case tea.KeyEnter, tea.KeyTab:
		if item, ok := m.input.PickerSelected(); ok {
			m.sendOutbound(ui.PickerSelectMsg{
				CallbackID: m.input.PickerCallbackID(),
				Value:      item.GetValue(),
				Accepted:   true,
			})
		} else {
			m.sendOutbound(ui.PickerSelectMsg{CallbackID: m.input.PickerCallbackID(), Accepted: false})
		}
		m.inputMode = ModeNormal
		m.input.HidePicker()
		m.recalculateLayout()
		return m, nil

	case tea.KeyRunes:
		m.input.PickerFilter(m.input.PickerQuery() + string(msg.Runes))
		return m, nil

	case tea.KeySpace:
		m.input.PickerFilter(m.input.PickerQuery() + " ")
		return m, nil

	case tea.KeyBackspace:
		query := m.input.PickerQuery()
		if len(query) > 0 {
			m.input.PickerFilter(query[:len(query)-1])
		}
		return m, nil
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

func (m *Model) sendOutbound(msg any) {
	if m.outbound == nil {
		return
	}
	select {
	case m.outbound <- msg:
	default:
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

func (m *Model) getLayout() ui.LayoutConfig {
	if len(m.luaLayout.Top) > 0 || len(m.luaLayout.Bottom) > 0 {
		return ui.LayoutConfig{
			Top:    m.luaLayout.Top,
			Bottom: m.luaLayout.Bottom,
		}
	}
	return ui.DefaultLayoutConfig()
}

// getWidget returns the Widget for a given name.
func (m *Model) getWidget(name string) widget.Widget {
	// Check widgets map (input, separator, bars)
	if w, ok := m.widgets[name]; ok {
		return w
	}

	// Panes (PaneManager returns *Pane which implements Widget)
	if m.panes.Exists(name) {
		return m.panes.Get(name)
	}

	return nil
}

// recalculateLayout computes widget sizes based on current dimensions and layout config.
// Called from Update handlers when window size or layout changes.
func (m *Model) recalculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	cfg := m.getLayout()

	topHeight := m.sizeDock(cfg.Top)
	bottomHeight := m.sizeDock(cfg.Bottom)

	m.viewportHeight = m.height - topHeight - bottomHeight
	if m.viewportHeight < 1 {
		m.viewportHeight = 1
	}
	m.viewport.SetSize(m.width, m.viewportHeight)
}

// sizeDock calculates sizes for dock widgets and returns total height.
// Sets widget sizes as a side effect.
func (m *Model) sizeDock(entries []ui.LayoutEntry) int {
	totalHeight := 0

	for _, entry := range entries {
		w := m.getWidget(entry.Name)
		if w == nil {
			continue
		}

		preferred := w.PreferredHeight()
		if preferred == 0 {
			continue
		}

		h := entry.Height
		if h == 0 {
			h = preferred
		}

		w.SetSize(m.width, h)
		totalHeight += h
	}

	return totalHeight
}

// renderDock renders a list of layout entries, returns combined view and total height.
// Assumes sizes have already been set by recalculateLayout.
func (m *Model) renderDock(entries []ui.LayoutEntry) (string, int) {
	var parts []string
	totalHeight := 0

	for _, entry := range entries {
		w := m.getWidget(entry.Name)
		if w == nil {
			continue
		}

		preferred := w.PreferredHeight()
		if preferred == 0 {
			continue
		}

		h := entry.Height
		if h == 0 {
			h = preferred
		}

		parts = append(parts, w.View())
		totalHeight += h
	}

	return strings.Join(parts, "\n"), totalHeight
}

// View implements tea.Model.
// This method should be pure - no side effects. All sizing is done in Update via recalculateLayout.
func (m Model) View() string {
	if !m.initialized {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	cfg := m.getLayout()

	topView, _ := m.renderDock(cfg.Top)
	bottomView, _ := m.renderDock(cfg.Bottom)

	var parts []string
	if topView != "" {
		parts = append(parts, topView)
	}
	parts = append(parts, m.viewport.View())
	if bottomView != "" {
		parts = append(parts, bottomView)
	}

	return strings.Join(parts, "\n")
}

func keyToString(msg tea.KeyMsg) string {
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		return string(msg.Runes)
	}

	switch msg.Type {
	case tea.KeyCtrlA:
		return "ctrl+a"
	case tea.KeyCtrlB:
		return "ctrl+b"
	case tea.KeyCtrlC:
		return "ctrl+c"
	case tea.KeyCtrlD:
		return "ctrl+d"
	case tea.KeyCtrlE:
		return "ctrl+e"
	case tea.KeyCtrlF:
		return "ctrl+f"
	case tea.KeyCtrlG:
		return "ctrl+g"
	case tea.KeyCtrlH:
		return "ctrl+h"
	case tea.KeyCtrlI:
		return "ctrl+i"
	case tea.KeyCtrlJ:
		return "ctrl+j"
	case tea.KeyCtrlK:
		return "ctrl+k"
	case tea.KeyCtrlL:
		return "ctrl+l"
	case tea.KeyCtrlM:
		return "ctrl+m"
	case tea.KeyCtrlN:
		return "ctrl+n"
	case tea.KeyCtrlO:
		return "ctrl+o"
	case tea.KeyCtrlP:
		return "ctrl+p"
	case tea.KeyCtrlQ:
		return "ctrl+q"
	case tea.KeyCtrlR:
		return "ctrl+r"
	case tea.KeyCtrlS:
		return "ctrl+s"
	case tea.KeyCtrlT:
		return "ctrl+t"
	case tea.KeyCtrlU:
		return "ctrl+u"
	case tea.KeyCtrlV:
		return "ctrl+v"
	case tea.KeyCtrlW:
		return "ctrl+w"
	case tea.KeyCtrlX:
		return "ctrl+x"
	case tea.KeyCtrlY:
		return "ctrl+y"
	case tea.KeyCtrlZ:
		return "ctrl+z"
	case tea.KeyF1:
		return "f1"
	case tea.KeyF2:
		return "f2"
	case tea.KeyF3:
		return "f3"
	case tea.KeyF4:
		return "f4"
	case tea.KeyF5:
		return "f5"
	case tea.KeyF6:
		return "f6"
	case tea.KeyF7:
		return "f7"
	case tea.KeyF8:
		return "f8"
	case tea.KeyF9:
		return "f9"
	case tea.KeyF10:
		return "f10"
	case tea.KeyF11:
		return "f11"
	case tea.KeyF12:
		return "f12"
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	default:
		return ""
	}
}
