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

// OverlayMode represents the current input overlay state.
type OverlayMode int

const (
	ModeNormal  OverlayMode = iota
	ModeSlash               // Slash command picker active
	ModeHistory             // Ctrl+R history search active
	ModeAlias               // Ctrl+T alias search active
)

// Item types for pickers (moved from overlay.go)

// commandItem wraps CommandInfo for picker use.
type commandItem struct {
	info CommandInfo
}

func (c commandItem) FilterValue() string {
	return c.info.Name + " " + c.info.Description
}

func (c commandItem) Render(width int, selected bool, matches []int, s style.Styles) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	// Highlight matched characters in name and description
	name := highlightText(c.info.Name, matches, 0, s)
	desc := ""
	if c.info.Description != "" {
		// Description starts at len(name)+1 (after the space)
		desc = " - " + highlightText(c.info.Description, matches, len(c.info.Name)+1, s)
	}

	line := "/" + name + desc

	if selected {
		return s.OverlaySelected.Render(prefix) + line
	}
	return s.OverlayNormal.Render(prefix) + line
}

// aliasItem wraps AliasInfo for picker use.
type aliasItem struct {
	info AliasInfo
}

func (a aliasItem) FilterValue() string {
	return a.info.Name
}

func (a aliasItem) Render(width int, selected bool, matches []int, s style.Styles) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	// Highlight matched characters in name
	name := highlightText(a.info.Name, matches, 0, s)

	// Format: name → value, truncated to fit
	// Arrow " → " is 3 visual chars
	nameLen := len([]rune(a.info.Name))
	arrowLen := 3
	availableForValue := width - nameLen - arrowLen - 4 // margin

	value := a.info.Value
	if availableForValue > 3 {
		valueRunes := []rune(value)
		if len(valueRunes) > availableForValue {
			value = string(valueRunes[:availableForValue-1]) + "…"
		}
	} else {
		value = "…"
	}

	line := name + " → " + value

	if selected {
		return s.OverlaySelected.Render(prefix) + line
	}
	return s.OverlayNormal.Render(prefix) + line
}

// historyItem wraps a history command for picker use.
type historyItem struct {
	command string
}

func (h historyItem) FilterValue() string {
	return h.command
}

func (h historyItem) Render(width int, selected bool, matches []int, s style.Styles) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	// Highlight matched characters
	line := highlightText(h.command, matches, 0, s)

	if selected {
		return s.OverlaySelected.Render(prefix + line)
	}
	return s.OverlayNormal.Render(prefix) + line
}

// highlightText highlights matched positions in text, with offset adjustment.
func highlightText(text string, positions []int, offset int, s style.Styles) string {
	if len(positions) == 0 {
		return text
	}

	textRunes := []rune(text)

	// Build set of positions relative to this text segment
	posSet := make(map[int]bool)
	for _, pos := range positions {
		relPos := pos - offset
		if relPos >= 0 && relPos < len(textRunes) {
			posSet[relPos] = true
		}
	}

	if len(posSet) == 0 {
		return text
	}

	var result strings.Builder
	for i, r := range textRunes {
		if posSet[i] {
			result.WriteString(s.OverlayMatch.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
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

	// Overlay state - Model owns mode and pickers directly
	overlayMode   OverlayMode
	slashPicker   *picker.Model[commandItem]
	historyPicker *picker.Model[historyItem]
	aliasPicker   *picker.Model[aliasItem]

	// History state - Model owns history directly
	history      []string
	historyIndex int    // -1 = draft, 0..n = history position
	historyLimit int    // Max history entries
	historyDraft string // Preserved when browsing history

	// Tab completion
	wordCache *util.CompletionEngine

	// Data provider (set by session)
	provider DataProvider

	// Layout provider (set by session, optional)
	layoutProvider layout.Provider

	// Cached Lua bar content (updated on tick)
	barCache map[string]layout.BarContent

	// State
	lastPrompt  string // For deduplication
	infobar     string // Lua-controlled info bar (above input)
	width       int
	height      int
	inputChan   chan<- string
	quitting    bool
	initialized bool

	// Pending lines for batched rendering
	pendingLines []string
}

// NewModel creates a new TUI model.
func NewModel(inputChan chan<- string, provider DataProvider) Model {
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
		// Use generic pickers directly - no wrapper structs
		slashPicker: picker.New[commandItem](picker.Config{
			MaxVisible: 8,
			EmptyText:  "No matching commands",
		}, styles),
		historyPicker: picker.New[historyItem](picker.Config{
			MaxVisible: 10,
			Header:     "History: ",
			EmptyText:  "No matches",
		}, styles),
		aliasPicker: picker.New[aliasItem](picker.Config{
			MaxVisible: 10,
			Header:     "Alias: ",
			EmptyText:  "No matches",
		}, styles),
		history:      make([]string, 0, 1000),
		historyIndex: -1,
		historyLimit: 10000,
		wordCache:    wordCache,
		inputChan:    inputChan,
		provider:     provider,
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
		return m, nil

	// Tick for batching and clock updates
	case tickMsg:
		// Flush any pending lines
		if len(m.pendingLines) > 0 {
			m.appendLines(m.pendingLines)
			m.pendingLines = nil
		}
		// Update Lua bar cache
		if m.layoutProvider != nil {
			m.barCache = m.layoutProvider.RenderBars(m.width)
		}
		return m, doTick()

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

	// Status text from Lua
	case StatusTextMsg:
		m.status.SetText(string(msg))
		return m, nil

	// Info bar from Lua
	case InfobarMsg:
		m.infobar = string(msg)
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
		if m.input.Value() == "" && m.overlayMode == ModeNormal {
			m.quitting = true
			return m, tea.Quit
		}
		// Clear input or cancel overlay
		if m.overlayMode != ModeNormal {
			m.overlayMode = ModeNormal
			m.slashPicker.Reset()
			m.historyPicker.Reset()
			m.aliasPicker.Reset()
			return m, nil
		}
		m.input.Reset()
		m.historyIndex = -1
		m.historyDraft = ""
		return m, nil

	case tea.KeyEsc:
		if m.overlayMode != ModeNormal {
			m.overlayMode = ModeNormal
			m.slashPicker.Reset()
			m.historyPicker.Reset()
			m.aliasPicker.Reset()
			return m, nil
		}
		m.input.Reset()
		m.historyIndex = -1
		m.historyDraft = ""
		return m, nil
	}

	// Mode-specific handling
	switch m.overlayMode {
	case ModeSlash:
		return m.handleSlashKey(msg)
	case ModeHistory:
		return m.handleHistoryKey(msg)
	case ModeAlias:
		return m.handleAliasKey(msg)
	default:
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Check Lua key bindings first
	// Note: This is called synchronously but Lua execution happens on Session goroutine.
	// For now we trust the provider to handle threading correctly.
	if m.layoutProvider != nil {
		keyStr := keyToString(msg)
		if keyStr != "" && m.layoutProvider.HandleKeyBind(keyStr) {
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

	switch msg.Type {
	case tea.KeyEnter:
		text := m.input.Value()
		if text != "" {
			m.addToHistory(text)
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
		m.historyIndex = -1
		m.historyDraft = ""
		return m, nil

	case tea.KeyUp:
		m.historyUp()
		m.updateSuggestions()
		return m, nil

	case tea.KeyDown:
		m.historyDown()
		m.updateSuggestions()
		return m, nil

	case tea.KeyCtrlU:
		m.input.SetValue("")
		return m, nil

	case tea.KeyCtrlW:
		m.deleteWord()
		return m, nil

	case tea.KeyCtrlR:
		m.overlayMode = ModeHistory
		m.setHistoryPickerItems()
		m.historyPicker.Filter("")
		return m, nil

	case tea.KeyCtrlT:
		m.overlayMode = ModeAlias
		if m.provider != nil {
			m.setAliasPickerItems(m.provider.Aliases())
		}
		m.aliasPicker.Filter("")
		return m, nil

	case tea.KeyPgUp:
		m.viewport.PageUp()
		m.status.SetScrollMode(m.viewport.Mode(), m.viewport.NewLineCount())
		return m, nil

	case tea.KeyPgDown:
		m.viewport.PageDown()
		m.status.SetScrollMode(m.viewport.Mode(), m.viewport.NewLineCount())
		return m, nil

	case tea.KeyEnd:
		m.viewport.GotoBottom()
		m.status.SetScrollMode(m.viewport.Mode(), m.viewport.NewLineCount())
		return m, nil

	case tea.KeyHome:
		m.viewport.GotoTop()
		m.status.SetScrollMode(m.viewport.Mode(), m.viewport.NewLineCount())
		return m, nil

	case tea.KeyRunes:
		// Check for "/" at start of empty input
		if len(msg.Runes) == 1 && msg.Runes[0] == '/' && m.input.Value() == "" {
			m.overlayMode = ModeSlash
			// Load commands via provider
			if m.provider != nil {
				m.setSlashPickerItems(m.provider.Commands())
			}
			m.slashPicker.Filter("")
		}
	}

	// Forward to input
	newInput, cmd := m.input.Update(msg)
	m.input = *newInput

	// Update suggestions and slash filter
	m.updateSuggestions()
	if m.overlayMode == ModeSlash {
		m.slashPicker.Filter(m.getSlashQuery())
	}

	return m, cmd
}

func (m Model) handleSlashKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.slashPicker.SelectUp()
		return m, nil

	case tea.KeyDown:
		m.slashPicker.SelectDown()
		return m, nil

	case tea.KeyEnter:
		// Get current input text
		text := m.input.Value()

		// Check if user has typed arguments (space after command)
		if strings.Contains(text, " ") {
			// User typed full command with args - submit as-is
			m.addToHistory(text)
			select {
			case m.inputChan <- text:
			default:
			}
		} else if item, ok := m.slashPicker.Selected(); ok {
			// No args - use selected command from picker
			fullCmd := "/" + item.info.Name
			m.addToHistory(fullCmd)
			select {
			case m.inputChan <- fullCmd:
			default:
			}
		}
		m.input.Reset()
		m.historyIndex = -1
		m.historyDraft = ""
		m.overlayMode = ModeNormal
		m.slashPicker.Reset()
		return m, nil

	case tea.KeyTab:
		// Insert command name and position cursor at end
		if item, ok := m.slashPicker.Selected(); ok {
			m.input.SetValue("/" + item.info.Name + " ")
			m.input.CursorEnd()
			m.input.ClearSuggestions()
		}
		m.overlayMode = ModeNormal
		m.slashPicker.Reset()
		return m, nil

	case tea.KeyBackspace:
		if m.input.Value() == "/" || m.input.Value() == "" {
			m.input.Reset()
			m.overlayMode = ModeNormal
			m.slashPicker.Reset()
			return m, nil
		}

	case tea.KeySpace:
		// Space commits to the selected command and exits picker mode
		// User can then type arguments freely
		if item, ok := m.slashPicker.Selected(); ok {
			m.input.SetValue("/" + item.info.Name + " ")
			m.input.CursorEnd()
			m.input.ClearSuggestions()
		}
		m.overlayMode = ModeNormal
		m.slashPicker.Reset()
		return m, nil
	}

	// Forward to input and update filter
	newInput, cmd := m.input.Update(msg)
	m.input = *newInput
	m.slashPicker.Filter(m.getSlashQuery())

	return m, cmd
}

func (m Model) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.historyPicker.SelectUp()
		return m, nil

	case tea.KeyDown:
		m.historyPicker.SelectDown()
		return m, nil

	case tea.KeyEnter, tea.KeyTab:
		// Insert selection
		if item, ok := m.historyPicker.Selected(); ok {
			m.input.SetValue(item.command)
		}
		m.overlayMode = ModeNormal
		m.historyPicker.Reset()
		return m, nil

	case tea.KeyCtrlR:
		// Toggle off
		m.overlayMode = ModeNormal
		m.historyPicker.Reset()
		return m, nil

	case tea.KeyRunes:
		// Add to search query
		m.historyPicker.Filter(m.historyPicker.Query() + string(msg.Runes))
		return m, nil

	case tea.KeySpace:
		// Add space to search query
		m.historyPicker.Filter(m.historyPicker.Query() + " ")
		return m, nil

	case tea.KeyBackspace:
		query := m.historyPicker.Query()
		if len(query) > 0 {
			m.historyPicker.Filter(query[:len(query)-1])
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleAliasKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.aliasPicker.SelectUp()
		return m, nil

	case tea.KeyDown:
		m.aliasPicker.SelectDown()
		return m, nil

	case tea.KeyEnter, tea.KeyTab:
		// Insert selected alias name
		if item, ok := m.aliasPicker.Selected(); ok {
			m.input.SetValue(item.info.Name)
			m.input.CursorEnd()
		}
		m.overlayMode = ModeNormal
		m.aliasPicker.Reset()
		return m, nil

	case tea.KeyCtrlT:
		// Toggle off
		m.overlayMode = ModeNormal
		m.aliasPicker.Reset()
		return m, nil

	case tea.KeyEsc:
		m.overlayMode = ModeNormal
		m.aliasPicker.Reset()
		return m, nil

	case tea.KeyRunes:
		// Add to search query
		m.aliasPicker.Filter(m.aliasPicker.Query() + string(msg.Runes))
		return m, nil

	case tea.KeySpace:
		// Add space to search query
		m.aliasPicker.Filter(m.aliasPicker.Query() + " ")
		return m, nil

	case tea.KeyBackspace:
		query := m.aliasPicker.Query()
		if len(query) > 0 {
			m.aliasPicker.Filter(query[:len(query)-1])
		}
		return m, nil
	}

	return m, nil
}

// History management (moved from input.go)

func (m *Model) addToHistory(cmd string) {
	if cmd == "" {
		return
	}
	// Don't add duplicates of the last command
	if len(m.history) > 0 && m.history[len(m.history)-1] == cmd {
		return
	}
	m.history = append(m.history, cmd)
	// Trim if over limit
	if len(m.history) > m.historyLimit {
		m.history = m.history[len(m.history)-m.historyLimit:]
	}
}

func (m *Model) historyUp() {
	if len(m.history) == 0 {
		return
	}

	if m.historyIndex == -1 {
		// Save current input as draft before entering history
		m.historyDraft = m.input.Value()
	}

	// If we have a prefix (draft), search for matching history
	if m.historyDraft != "" {
		start := m.historyIndex - 1
		if m.historyIndex == -1 {
			start = len(m.history) - 1
		}
		for i := start; i >= 0; i-- {
			if strings.HasPrefix(m.history[i], m.historyDraft) {
				m.historyIndex = i
				m.input.SetValue(m.history[i])
				m.input.CursorEnd()
				return
			}
		}
		// No match found, stay where we are
		return
	}

	// No prefix - cycle through all history
	if m.historyIndex == -1 {
		m.historyIndex = len(m.history) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}

	m.input.SetValue(m.history[m.historyIndex])
	m.input.CursorEnd()
}

func (m *Model) historyDown() {
	if m.historyIndex == -1 {
		return // Already at draft
	}

	// If we have a prefix (draft), search for matching history
	if m.historyDraft != "" {
		for i := m.historyIndex + 1; i < len(m.history); i++ {
			if strings.HasPrefix(m.history[i], m.historyDraft) {
				m.historyIndex = i
				m.input.SetValue(m.history[i])
				m.input.CursorEnd()
				return
			}
		}
		// No more matches - return to draft
		m.historyIndex = -1
		m.input.SetValue(m.historyDraft)
		m.input.CursorEnd()
		return
	}

	// No prefix - cycle through all history
	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.input.SetValue(m.history[m.historyIndex])
	} else {
		// Return to draft
		m.historyIndex = -1
		m.input.SetValue(m.historyDraft)
	}
	m.input.CursorEnd()
}

// Completion and suggestions (moved from input.go)

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

// Picker helpers

func (m *Model) setSlashPickerItems(commands []CommandInfo) {
	items := make([]commandItem, len(commands))
	for i, cmd := range commands {
		items[i] = commandItem{info: cmd}
	}
	m.slashPicker.SetItems(items)
}

func (m *Model) setHistoryPickerItems() {
	// Deduplicate: keep only most recent occurrence of each command
	seen := make(map[string]bool)
	deduped := make([]historyItem, 0, len(m.history))

	// Iterate backwards (most recent first)
	for i := len(m.history) - 1; i >= 0; i-- {
		cmd := m.history[i]
		if !seen[cmd] {
			seen[cmd] = true
			deduped = append(deduped, historyItem{command: cmd})
		}
	}

	m.historyPicker.SetItems(deduped)
}

func (m *Model) setAliasPickerItems(aliases []AliasInfo) {
	items := make([]aliasItem, len(aliases))
	for i, alias := range aliases {
		items[i] = aliasItem{info: alias}
	}
	m.aliasPicker.SetItems(items)
}

func (m *Model) getSlashQuery() string {
	val := m.input.Value()
	if strings.HasPrefix(val, "/") {
		return val[1:]
	}
	return ""
}

func (m *Model) appendLines(lines []string) {
	for _, line := range lines {
		m.scrollback.Append(line)
	}
	m.viewport.OnNewLines(len(lines))
	m.status.SetScrollMode(m.viewport.Mode(), m.viewport.NewLineCount())
}

// SetLayoutProvider sets the layout provider for custom layouts.
func (m *Model) SetLayoutProvider(lp layout.Provider) {
	m.layoutProvider = lp
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
	m.slashPicker.SetWidth(m.width)
	m.historyPicker.SetWidth(m.width)
	m.aliasPicker.SetWidth(m.width)
}

// getLayout returns the current layout, using default if no provider.
func (m *Model) getLayout() layout.Config {
	if m.layoutProvider != nil {
		return m.layoutProvider.Layout()
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
	// Built-in components
	switch name {
	case "input":
		// separator + input + separator, plus overlay when active
		h := 3 // top separator + input + bottom separator
		switch m.overlayMode {
		case ModeSlash:
			h += m.slashPicker.Height()
		case ModeHistory:
			h += m.historyPicker.Height()
		case ModeAlias:
			h += m.aliasPicker.Height()
		}
		return h
	case "status":
		return 1
	case "separator":
		return 1
	case "infobar":
		if m.infobar != "" {
			return 1
		}
		return 0 // Hidden when empty
	}

	// Custom bar
	if m.layoutProvider != nil {
		if bar := m.layoutProvider.Bar(name); bar != nil {
			h := 1 // bar content
			switch bar.Border {
			case layout.BorderTop, layout.BorderBottom:
				h += 1
			case layout.BorderBoth:
				h += 2
			}
			return h
		}
	}

	// Custom pane
	if m.layoutProvider != nil {
		if pane := m.layoutProvider.Pane(name); pane != nil && pane.Visible {
			h := pane.Height
			if pane.Title {
				h += 1
			}
			switch pane.Border {
			case layout.BorderTop, layout.BorderBottom:
				h += 1
			case layout.BorderBoth:
				h += 2
			}
			return h
		}
	}

	return 0
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.initialized {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	// Recalculate viewport height (overlay is part of input component)
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

	// 3. Bottom dock components (overlay renders as part of "input")
	for _, name := range layoutCfg.Bottom {
		if rendered := m.renderComponent(name); rendered != "" {
			parts = append(parts, rendered)
		}
	}

	return strings.Join(parts, "\n")
}

// renderComponent renders a component by name.
func (m Model) renderComponent(name string) string {
	// Built-in components
	switch name {
	case "input":
		// Overlay (if active) > separator > input > separator
		var parts []string
		switch m.overlayMode {
		case ModeSlash:
			parts = append(parts, m.slashPicker.View())
		case ModeHistory:
			parts = append(parts, m.historyPicker.View())
		case ModeAlias:
			parts = append(parts, m.aliasPicker.View())
		}
		parts = append(parts, m.borderLine())
		parts = append(parts, m.input.View())
		parts = append(parts, m.borderLine())
		return strings.Join(parts, "\n")
	case "status":
		return m.status.View()
	case "separator":
		return m.borderLine()
	case "infobar":
		if m.infobar != "" {
			return m.infobar
		}
		return "" // Hidden when empty
	}

	// Lua-defined bar (from cache)
	if content, ok := m.barCache[name]; ok {
		return m.renderBarContent(content)
	}

	// Go-defined custom bar (legacy)
	if m.layoutProvider != nil {
		if bar := m.layoutProvider.Bar(name); bar != nil {
			return m.renderBar(bar)
		}
	}

	// Custom pane
	if m.layoutProvider != nil {
		if pane := m.layoutProvider.Pane(name); pane != nil && pane.Visible {
			return m.renderPane(name, pane)
		}
	}

	return ""
}

// renderBar renders a single bar with optional borders.
func (m Model) renderBar(bar *layout.BarDef) string {
	var parts []string

	// Top border
	if bar.Border == layout.BorderTop || bar.Border == layout.BorderBoth {
		parts = append(parts, m.borderLine())
	}

	// Bar content
	state := layout.ClientState{}
	if m.layoutProvider != nil {
		state = m.layoutProvider.State()
	}
	content := bar.Render(state, m.width)
	parts = append(parts, m.renderBarContent(content))

	// Bottom border
	if bar.Border == layout.BorderBottom || bar.Border == layout.BorderBoth {
		parts = append(parts, m.borderLine())
	}

	return strings.Join(parts, "\n")
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
		leftPad := (m.width - centerLen) / 2 - leftLen
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

// renderPane renders a pane with optional title and borders.
func (m Model) renderPane(name string, pane *layout.PaneDef) string {
	var parts []string

	// Top border
	if pane.Border == layout.BorderTop || pane.Border == layout.BorderBoth {
		parts = append(parts, m.borderLine())
	}

	// Title
	if pane.Title {
		title := m.styles.PaneHeader.Render(" " + name + " ")
		titlePad := m.width - visibleLen(title)
		if titlePad > 0 {
			title += "\x1b[90m" + strings.Repeat("─", titlePad) + "\x1b[0m"
		}
		parts = append(parts, title)
	}

	// Content - show last N lines
	lines := m.layoutProvider.PaneLines(name)
	height := pane.Height
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}

	// Pad or fill to exact height
	for i := 0; i < height; i++ {
		if i < len(lines) {
			parts = append(parts, lines[i])
		} else {
			parts = append(parts, "")
		}
	}

	// Bottom border
	if pane.Border == layout.BorderBottom || pane.Border == layout.BorderBoth {
		parts = append(parts, m.borderLine())
	}

	return strings.Join(parts, "\n")
}

// visibleLen returns string length ignoring ANSI escape codes.
func visibleLen(s string) int {
	return util.VisibleLen(s)
}

// keyToString converts a Bubble Tea key message to a normalized string.
// Returns empty string for keys we don't want to expose to Lua.
func keyToString(msg tea.KeyMsg) string {
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
	default:
		return ""
	}
}
