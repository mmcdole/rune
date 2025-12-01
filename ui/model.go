package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui/components/input"
	"github.com/drake/rune/ui/components/panes"
	"github.com/drake/rune/ui/components/picker"
	"github.com/drake/rune/ui/components/status"
	"github.com/drake/rune/ui/components/viewport"
	"github.com/drake/rune/ui/layout"
	"github.com/drake/rune/ui/style"
	"github.com/drake/rune/ui/util"
)

// filterClearSequences removes ANSI sequences that would clear the screen.
// MUD clients typically ignore these to prevent server-side screen wipes.
func filterClearSequences(line string) string {
	line = strings.ReplaceAll(line, "\x1b[2J", "")   // Clear entire screen
	line = strings.ReplaceAll(line, "\x1b[H", "")    // Move cursor to home
	line = strings.ReplaceAll(line, "\x1b[0;0H", "") // Move cursor to 0,0
	line = strings.ReplaceAll(line, "\x1b[1;1H", "") // Move cursor to 1,1
	return line
}

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	// Display components
	scrollback *viewport.ScrollbackBuffer
	viewport   *viewport.Viewport
	input      input.CommandPrompt
	status     status.Bar
	panes      *panes.Manager
	styles     style.Styles

	// Generic picker (replaces slashPicker, historyPicker, aliasPicker)
	picker       *picker.Model[GenericItem]
	pickerActive bool
	pickerCB     string // Current callback ID for picker selection
	pickerInline bool   // True if picker is in inline mode (filters based on input content)

	// Tab completion
	wordCache *util.CompletionEngine

	// Push-based state from Session (thread-safe local caches)
	boundKeys  map[string]bool       // Keys bound in Lua
	barContent map[string]BarContent // Rendered bar content from Lua
	luaLayout  struct {              // Layout configuration from Lua
		Top    []string
		Bottom []string
	}

	// State
	lastPrompt string // For deduplication
	width      int
	height      int
	inputChan   chan<- string
	outbound    chan<- any // Messages from UI to Session
	quitting    bool
	initialized bool

	// Pending lines for batched rendering
	pendingLines []string
}

// NewModel creates a new TUI model.
func NewModel(inputChan chan<- string, outbound chan<- any) Model {
	styles := style.DefaultStyles()
	scrollback := viewport.NewScrollbackBuffer(100000)
	vp := viewport.New(scrollback)
	wordCache := util.NewCompletionEngine(5000) // Remember last 5000 unique words

	return Model{
		scrollback: scrollback,
		viewport:   vp,
		input:      input.New(),
		status:     status.New(styles),
		panes:      panes.NewManager(styles),
		styles:     styles,
		// Single generic picker
		picker: picker.New[GenericItem](picker.Config{
			MaxVisible: 10,
			EmptyText:  "No matches",
		}, styles),
		wordCache: wordCache,
		inputChan:    inputChan,
		outbound:     outbound,
	}
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

	// Window size
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateDimensions()
		m.initialized = true
		// Notify Session of size change (for rune.state.width/height)
		m.sendOutbound(WindowSizeChangedMsg{Width: msg.Width, Height: msg.Height})
		return m, nil

	// Tick for batching and clock updates
	case tickMsg:
		// Flush any pending lines
		if len(m.pendingLines) > 0 {
			m.appendLines(m.pendingLines)
			m.pendingLines = nil
		}
		// Note: Bar content is now pushed by Session via UpdateBarsMsg
		return m, doTick()

	// Push-based updates from Session (thread-safe)
	case UpdateBindsMsg:
		m.boundKeys = msg
		return m, nil

	case UpdateBarsMsg:
		m.barContent = msg
		return m, nil

	case UpdateLayoutMsg:
		m.luaLayout.Top = msg.Top
		m.luaLayout.Bottom = msg.Bottom
		m.updateDimensions()
		return m, nil

	case ShowPickerMsg:
		m.picker.SetItems(msg.Items)
		m.pickerCB = msg.CallbackID
		m.pickerActive = true
		m.pickerInline = msg.Inline

		if msg.Inline {
			// Inline mode: User types in main input, picker filters passively.
			// Hide the picker's internal header to avoid duplicate search UI.
			m.picker.SetHeader("")
			// Filter immediately based on current input
			m.picker.Filter(m.input.Value())
		} else {
			// Modal mode: Picker traps keys and shows its own search header.
			// Add ": " suffix for the search prompt display (e.g., "History: query█")
			header := msg.Title
			if header != "" {
				header += ": "
			}
			m.picker.SetHeader(header)
			m.picker.Filter("") // Reset filter for modal mode
		}
		return m, nil

	case SetInputMsg:
		m.input.SetValue(string(msg))
		m.input.CursorEnd()
		return m, nil

	// Batched lines from aggregator
	case flushLinesMsg:
		cleanLines := make([]string, len(msg.Lines))
		for i, line := range msg.Lines {
			cleanLines[i] = filterClearSequences(line)
			m.wordCache.AddLine(cleanLines[i]) // Feed tab completion cache
		}
		m.appendLines(cleanLines)
		return m, nil

	// General display line (server output or prompt commit)
	case DisplayLineMsg:
		cleanLine := filterClearSequences(string(msg))
		m.wordCache.AddLine(cleanLine)
		m.appendLines([]string{cleanLine})
		return m, nil

	// Single server line - batch for next tick
	case ServerLineMsg:
		cleanLine := filterClearSequences(string(msg))
		m.pendingLines = append(m.pendingLines, cleanLine)
		m.wordCache.AddLine(cleanLine) // Feed tab completion cache
		return m, nil

	// Echo line
	case EchoLineMsg:
		m.appendLines([]string{string(msg)})
		return m, nil

	// Server prompt (partial line)
	case PromptMsg:
		text := string(msg)
		if text != m.lastPrompt {
			m.viewport.SetPrompt(text)
			m.lastPrompt = text
		}
		return m, nil

	// Connection state
	case ConnectionStateMsg:
		m.status.SetConnectionState(status.ConnectionState(msg.State), msg.Address)
		return m, nil

	// Pane operations from Lua
	case PaneCreateMsg:
		m.panes.Create(msg.Name)
		return m, nil

	case PaneWriteMsg:
		m.panes.Write(msg.Name, msg.Text)
		return m, nil

	case PaneToggleMsg:
		m.panes.Toggle(msg.Name)
		m.updateDimensions()
		return m, nil

	case PaneClearMsg:
		m.panes.Clear(msg.Name)
		return m, nil

	case PaneBindMsg:
		m.panes.BindKey(msg.Key, msg.Name)
		return m, nil

	// Key handling
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.input.Value() == "" && !m.pickerActive {
			m.quitting = true
			return m, tea.Quit
		}
		// Clear input or cancel picker
		if m.pickerActive {
			m.pickerActive = false
			m.picker.Reset()
			m.sendOutbound(PickerSelectMsg{CallbackID: m.pickerCB, Accepted: false})
			return m, nil
		}
		m.input.Reset()
		return m, nil

	case tea.KeyEsc:
		if m.pickerActive {
			m.pickerActive = false
			m.picker.Reset()
			m.sendOutbound(PickerSelectMsg{CallbackID: m.pickerCB, Accepted: false})
			return m, nil
		}
		m.input.Reset()
		return m, nil
	}

	// Modal picker handling (non-linked mode traps all keys)
	if m.pickerActive && !m.pickerInline {
		return m.handlePickerKey(msg)
	}

	// Normal input (includes linked picker mode)
	return m.handleNormalKey(msg)
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Check Lua key bindings (using local cache - thread-safe)
	keyStr := keyToString(msg)
	if keyStr != "" && m.boundKeys[keyStr] {
		// Don't intercept Up/Down when inline picker is active - let picker handle them
		if m.pickerActive && m.pickerInline && (keyStr == "up" || keyStr == "down") {
			// Fall through to picker handling below
		} else {
			// Key is bound in Lua - send to Session for execution
			m.sendOutbound(ExecuteBindMsg(keyStr))
			return m, nil
		}
	}

	// Check for bound pane toggle keys (only when input is empty)
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && m.input.Value() == "" {
		key := string(msg.Runes)
		if m.panes.HandleKey(key) {
			m.updateDimensions()
			return m, nil
		}
	}

	// Linked picker mode: Up/Down navigate picker, Tab completes
	if m.pickerActive && m.pickerInline {
		switch msg.Type {
		case tea.KeyUp:
			m.picker.SelectUp()
			return m, nil
		case tea.KeyDown:
			m.picker.SelectDown()
			return m, nil
		case tea.KeyTab:
			// Tab auto-completes the selection
			if item, ok := m.picker.Selected(); ok {
				m.input.SetValue(item.Value + " ")
				m.input.CursorEnd()
				m.sendOutbound(PickerSelectMsg{
					CallbackID: m.pickerCB,
					Value:      item.Value,
					Accepted:   true,
				})
			}
			m.pickerActive = false
			m.pickerInline = false
			m.picker.Reset()
			return m, nil
		case tea.KeyEnter:
			// Enter accepts selection OR submits current input
			if item, ok := m.picker.Selected(); ok {
				m.sendOutbound(PickerSelectMsg{
					CallbackID: m.pickerCB,
					Value:      item.Value,
					Accepted:   true,
				})
			}
			m.pickerActive = false
			m.pickerInline = false
			m.picker.Reset()
			// Fall through to normal Enter handling below
		}
	}

	switch msg.Type {
	case tea.KeyEnter:
		text := m.input.Value()
		if text != "" {
			m.wordCache.AddInput(text) // Feed user input to completion cache (preserves punctuation)
		}
		// Send to orchestrator (including empty string for blank enter)
		select {
		case m.inputChan <- text:
		default:
			// Channel full, append warning to scrollback
			m.scrollback.Append("\033[31m[WARNING] Input dropped - engine lagging\033[0m")
		}
		m.input.Reset()
		return m, nil

	case tea.KeyCtrlU:
		m.input.SetValue("")
		m.updateInlinePicker()
		return m, nil

	case tea.KeyCtrlW:
		m.deleteWord()
		m.updateInlinePicker()
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
	newInput, cmd := m.input.Update(msg)
	m.input = *newInput

	// Notify Session if input content changed (for rune.input.get())
	if newValue := m.input.Value(); newValue != oldValue {
		m.sendOutbound(InputChangedMsg(newValue))
	}

	// Update linked picker filter based on new input content
	m.updateInlinePicker()

	// Update suggestions
	m.updateSuggestions()

	return m, cmd
}

// updateInlinePicker updates the inline picker filter based on input content.
// Closes the picker if input becomes empty.
func (m *Model) updateInlinePicker() {
	if !m.pickerActive || !m.pickerInline {
		return
	}

	val := m.input.Value()

	// If input is empty, close picker
	if val == "" {
		m.pickerActive = false
		m.pickerInline = false
		m.sendOutbound(PickerSelectMsg{CallbackID: m.pickerCB, Accepted: false})
		m.picker.Reset()
		return
	}

	// Update filter based on full input content
	m.picker.Filter(val)
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.picker.SelectUp()
		return m, nil

	case tea.KeyDown:
		m.picker.SelectDown()
		return m, nil

	case tea.KeyEnter, tea.KeyTab:
		// Accept selection
		if item, ok := m.picker.Selected(); ok {
			m.sendOutbound(PickerSelectMsg{
				CallbackID: m.pickerCB,
				Value:      item.Value,
				Accepted:   true,
			})
		} else {
			m.sendOutbound(PickerSelectMsg{CallbackID: m.pickerCB, Accepted: false})
		}
		m.pickerActive = false
		m.picker.Reset()
		return m, nil

	case tea.KeyRunes:
		// Add to search query
		m.picker.Filter(m.picker.Query() + string(msg.Runes))
		return m, nil

	case tea.KeySpace:
		// Add space to search query
		m.picker.Filter(m.picker.Query() + " ")
		return m, nil

	case tea.KeyBackspace:
		query := m.picker.Query()
		if len(query) > 0 {
			m.picker.Filter(query[:len(query)-1])
		}
		return m, nil
	}

	return m, nil
}

// Completion and suggestions

func (m *Model) updateSuggestions() {
	if m.wordCache == nil {
		return
	}

	val := m.input.Value()
	if val == "" {
		m.input.SetSuggestions(nil)
		return
	}

	// Find the word at/before cursor
	pos := m.input.Position()
	start, end := util.FindWordBoundaries(val, pos)
	if start == end {
		m.input.SetSuggestions(nil)
		return
	}

	prefix := val[start:end]
	if len(prefix) < 2 {
		m.input.SetSuggestions(nil)
		return
	}

	matches := m.wordCache.FindMatches(prefix)
	if len(matches) == 0 {
		m.input.SetSuggestions(nil)
		return
	}

	// Build full-line suggestions by replacing the current word
	before := val[:start]
	after := ""
	if end < len(val) {
		after = val[end:]
	}

	suggestions := make([]string, 0, len(matches))
	for _, match := range matches {
		suggestions = append(suggestions, before+match+after)
	}

	m.input.SetSuggestions(suggestions)
}

func (m *Model) deleteWord() {
	val := m.input.Value()
	pos := m.input.Position()
	if pos > 0 {
		newPos := pos - 1
		for newPos > 0 && val[newPos-1] == ' ' {
			newPos--
		}
		for newPos > 0 && val[newPos-1] != ' ' {
			newPos--
		}
		m.input.SetValue(val[:newPos] + val[pos:])
		m.input.SetCursor(newPos)
	}
}

func (m *Model) appendLines(lines []string) {
	for _, line := range lines {
		m.scrollback.Append(line)
	}
	m.viewport.OnNewLines(len(lines))
	m.updateScrollState()
}


// sendOutbound sends a message to Session via the outbound channel.
// Non-blocking - drops message if channel is full.
func (m *Model) sendOutbound(msg any) {
	if m.outbound == nil {
		return
	}
	select {
	case m.outbound <- msg:
	default:
		// Drop rather than block UI
	}
}

// updateScrollState updates the local status bar and notifies Session.
func (m *Model) updateScrollState() {
	mode := m.viewport.Mode()
	newLines := m.viewport.NewLineCount()

	// Update local status component (fallback)
	m.status.SetScrollMode(mode, newLines)

	// Notify Session to update rune.state
	modeStr := "live"
	if mode != 0 { // ModeScrolled
		modeStr = "scrolled"
	}
	m.sendOutbound(ScrollStateChangedMsg{Mode: modeStr, NewLines: newLines})
}

func (m *Model) updateDimensions() {
	layoutCfg := m.getLayout()

	// Calculate dock heights from layout (includes overlay as part of input)
	topHeight := m.dockHeight(layoutCfg.Top)
	bottomHeight := m.dockHeight(layoutCfg.Bottom)

	// Viewport gets remaining space
	viewportHeight := m.height - topHeight - bottomHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	m.viewport.SetDimensions(m.width, viewportHeight)
	m.input.SetWidth(m.width)
	m.status.SetWidth(m.width)
	m.panes.SetWidth(m.width)
	m.picker.SetWidth(m.width)
}

// getLayout returns the current layout configuration.
// Uses pushed layout if set, otherwise falls back to default.
func (m *Model) getLayout() layout.Config {
	if len(m.luaLayout.Top) > 0 || len(m.luaLayout.Bottom) > 0 {
		return layout.Config{
			Top:    m.luaLayout.Top,
			Bottom: m.luaLayout.Bottom,
		}
	}
	return layout.DefaultConfig()
}

// borderLine returns a dim horizontal line spanning the full width.
func (m Model) borderLine() string {
	return "\x1b[90m" + strings.Repeat("─", m.width) + "\x1b[0m"
}

// dockHeight calculates total height of components in a dock.
func (m *Model) dockHeight(components []string) int {
	height := 0
	for _, name := range components {
		height += m.componentHeight(name)
	}
	return height
}

// componentHeight returns the height of a single component.
func (m *Model) componentHeight(name string) int {
	switch name {
	case "input":
		// separator + input + separator, plus picker overlay when active
		h := 3 // top separator + input + bottom separator
		if m.pickerActive {
			h += m.picker.Height()
		}
		return h
	case "status":
		return 1
	case "separator":
		return 1
	default:
		// Custom Lua bar (all bars are 1 line)
		if _, ok := m.barContent[name]; ok {
			return 1
		}
		return 0
	}
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.initialized {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	// Recalculate viewport height (picker is part of input component)
	layoutCfg := m.getLayout()
	topHeight := m.dockHeight(layoutCfg.Top)
	bottomHeight := m.dockHeight(layoutCfg.Bottom)
	viewportHeight := m.height - topHeight - bottomHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.SetDimensions(m.width, viewportHeight)

	var parts []string

	// 1. Top dock components
	for _, name := range layoutCfg.Top {
		if rendered := m.renderComponent(name); rendered != "" {
			parts = append(parts, rendered)
		}
	}

	// 2. Main viewport (scrollback)
	parts = append(parts, m.viewport.View())

	// 3. Bottom dock components (picker renders as part of "input")
	for _, name := range layoutCfg.Bottom {
		if rendered := m.renderComponent(name); rendered != "" {
			parts = append(parts, rendered)
		}
	}

	return strings.Join(parts, "\n")
}

// renderComponent renders a component by name.
func (m Model) renderComponent(name string) string {
	// Check pushed bar content first (allows overriding built-ins like "status")
	if content, ok := m.barContent[name]; ok {
		return m.renderBarContent(content)
	}

	// Built-in components (fallback if no bar defined)
	switch name {
	case "input":
		// Picker overlay (if active) > separator > input > separator
		var parts []string
		if m.pickerActive {
			parts = append(parts, m.picker.View())
		}
		parts = append(parts, m.borderLine())
		parts = append(parts, m.input.View())
		parts = append(parts, m.borderLine())
		return strings.Join(parts, "\n")
	case "status":
		return m.status.View()
	case "separator":
		return m.borderLine()
	}

	return ""
}

// renderBarContent renders BarContent with left/center/right alignment.
func (m Model) renderBarContent(content layout.BarContent) string {
	left := content.Left
	center := content.Center
	right := content.Right

	leftLen := visibleLen(left)
	centerLen := visibleLen(center)
	rightLen := visibleLen(right)

	// Calculate spacing
	if center != "" {
		// Three-part layout: left ... center ... right
		leftPad := (m.width-centerLen)/2 - leftLen
		if leftPad < 1 {
			leftPad = 1
		}
		rightPad := m.width - leftLen - leftPad - centerLen - rightLen
		if rightPad < 1 {
			rightPad = 1
		}
		return left + strings.Repeat(" ", leftPad) + center + strings.Repeat(" ", rightPad) + right
	}

	// Two-part layout: left ... right
	pad := m.width - leftLen - rightLen
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// visibleLen returns string length ignoring ANSI escape codes.
func visibleLen(s string) int {
	return util.VisibleLen(s)
}

// keyToString converts a Bubble Tea key message to a normalized string.
// Returns empty string for keys we don't want to expose to Lua.
func keyToString(msg tea.KeyMsg) string {
	// Handle regular character keys (e.g., "j", "G", "?")
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
