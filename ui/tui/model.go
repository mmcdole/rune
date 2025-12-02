package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/tui/layout"
	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/widget"
)

// tickMsg is used for periodic updates (line batching, clock refresh).
type tickMsg time.Time

// flushLinesMsg signals the model to flush pending lines.
type flushLinesMsg struct {
	Lines []string
}

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

// filterClearSequences removes ANSI sequences that would clear the screen.
func filterClearSequences(line string) string {
	line = strings.ReplaceAll(line, "\x1b[2J", "")
	line = strings.ReplaceAll(line, "\x1b[H", "")
	line = strings.ReplaceAll(line, "\x1b[0;0H", "")
	line = strings.ReplaceAll(line, "\x1b[1;1H", "")
	return line
}

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	// Layout
	engine    *layout.Engine
	renderers map[string]layout.Renderer // static: input, status, separator
	bars      map[string]*widget.Bar     // created when barContent changes

	// Widgets
	scrollback *widget.ScrollbackBuffer
	viewport   *widget.Viewport
	input      *widget.Input
	status     *widget.Status
	panes      *widget.PaneManager
	styles     style.Styles

	// Input mode
	inputMode InputMode

	// Push-based state from Session
	boundKeys  map[string]bool
	barContent map[string]ui.BarContent
	luaLayout  struct {
		Top    []string
		Bottom []string
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
}

// NewModel creates a new TUI model.
func NewModel(inputChan chan<- string, outbound chan<- any) Model {
	styles := style.DefaultStyles()
	scrollback := widget.NewScrollbackBuffer(100000)
	viewport := widget.NewViewport(scrollback)
	input := widget.NewInput(styles)
	status := widget.NewStatus(styles)
	panes := widget.NewPaneManager(styles)

	m := Model{
		engine:     layout.NewEngine(),
		scrollback: scrollback,
		viewport:   viewport,
		input:      input,
		status:     status,
		panes:      panes,
		styles:     styles,
		inputChan:  inputChan,
		outbound:   outbound,
		renderers:  make(map[string]layout.Renderer),
		bars:       make(map[string]*widget.Bar),
	}

	// Register static renderers (all implement layout.Renderer)
	m.renderers["input"] = input
	m.renderers["status"] = status
	m.renderers["separator"] = widget.NewSeparator()

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
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.engine.SetSize(msg.Width, msg.Height)
		m.initialized = true
		m.sendOutbound(ui.WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
		return m, nil

	case tickMsg:
		if len(m.pendingLines) > 0 {
			m.appendLines(m.pendingLines)
			m.pendingLines = nil
		}
		return m, doTick()

	case ui.UpdateBindsMsg:
		m.boundKeys = msg
		return m, nil

	case ui.UpdateBarsMsg:
		m.barContent = msg
		return m, nil

	case ui.UpdateLayoutMsg:
		m.luaLayout.Top = msg.Top
		m.luaLayout.Bottom = msg.Bottom
		return m, nil

	case ui.ShowPickerMsg:
		if msg.Inline {
			m.inputMode = ModePickerInline
		} else {
			m.inputMode = ModePickerModal
		}
		m.input.ShowPicker(msg.Items, msg.Title, msg.CallbackID, msg.Inline)
		return m, nil

	case ui.SetInputMsg:
		m.input.SetValue(string(msg))
		m.input.CursorEnd()
		return m, nil

	case flushLinesMsg:
		cleanLines := make([]string, len(msg.Lines))
		for i, line := range msg.Lines {
			cleanLines[i] = filterClearSequences(line)
			m.input.AddToWordCache(cleanLines[i])
		}
		m.appendLines(cleanLines)
		return m, nil

	case ui.PrintLineMsg:
		cleanLine := filterClearSequences(string(msg))
		m.pendingLines = append(m.pendingLines, cleanLine)
		m.input.AddToWordCache(cleanLine)
		return m, nil

	case ui.EchoLineMsg:
		m.appendLines([]string{string(msg)})
		return m, nil

	case ui.PromptMsg:
		text := string(msg)
		if text != m.lastPrompt {
			m.viewport.SetPrompt(text)
			m.lastPrompt = text
		}
		return m, nil

	case ui.ConnectionStateMsg:
		m.status.SetConnectionState(msg.State, msg.Address)
		return m, nil

	case ui.PaneCreateMsg:
		m.panes.Create(msg.Name)
		return m, nil

	case ui.PaneWriteMsg:
		m.panes.Write(msg.Name, msg.Text)
		return m, nil

	case ui.PaneToggleMsg:
		m.panes.Toggle(msg.Name)
		return m, nil

	case ui.PaneClearMsg:
		m.panes.Clear(msg.Name)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, tea.Batch(cmds...)
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
	m.input.Update(msg)

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
	m.input.Update(msg)

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

	m.status.SetScrollMode(mode, newLines)

	modeStr := "live"
	if mode != 0 {
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

// getRenderer returns the layout.Renderer for a component name.
func (m *Model) getRenderer(name string) layout.Renderer {
	// Static registry (input, status, separator)
	if r, ok := m.renderers[name]; ok {
		return r
	}

	// Bars (created lazily, cached)
	if _, ok := m.barContent[name]; ok {
		if _, exists := m.bars[name]; !exists {
			m.bars[name] = widget.NewBar(name, &m.barContent)
		}
		return m.bars[name]
	}

	// Panes (PaneManager returns *Pane which implements layout.Renderer)
	if m.panes.Exists(name) {
		return m.panes.Get(name)
	}

	return nil
}

// buildDock converts layout names to a Dock of renderers.
func (m *Model) buildDock(names []string) *layout.Dock {
	dock := &layout.Dock{}
	for _, name := range names {
		if r := m.getRenderer(name); r != nil {
			dock.Renderers = append(dock.Renderers, r)
		}
	}
	return dock
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.initialized {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	cfg := m.getLayout()
	top := m.buildDock(cfg.Top)
	bottom := m.buildDock(cfg.Bottom)

	viewportHeight := m.engine.Calculate(top, bottom)
	m.viewport.SetDimensions(m.engine.Width(), viewportHeight)

	var parts []string
	if v := top.View(); v != "" {
		parts = append(parts, v)
	}
	parts = append(parts, m.viewport.View())
	if v := bottom.View(); v != "" {
		parts = append(parts, v)
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
