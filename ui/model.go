package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	// Display components
	scrollback *ScrollbackBuffer
	viewport   *ScrollbackViewport
	input      InputModel
	status     StatusBar
	panes      *PaneManager
	styles     Styles

	// Overlays
	slashPicker SlashPicker
	fuzzySearch FuzzySearch

	// Tab completion
	wordCache *WordCache

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
func NewModel(inputChan chan<- string) Model {
	styles := DefaultStyles()
	scrollback := NewScrollbackBuffer(100000)
	viewport := NewScrollbackViewport(scrollback)
	wordCache := NewWordCache(5000) // Remember last 5000 unique words

	return Model{
		scrollback:  scrollback,
		viewport:    viewport,
		input:       NewInputModel(wordCache),
		status:      NewStatusBar(styles),
		panes:       NewPaneManager(styles),
		styles:      styles,
		slashPicker: NewSlashPicker(styles),
		fuzzySearch: NewFuzzySearch(styles),
		wordCache:   wordCache,
		inputChan:   inputChan,
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
		// Update status bar clock
		return m, doTick()

	// Batched lines from aggregator
	case flushLinesMsg:
		for _, line := range msg.Lines {
			m.wordCache.AddLine(line) // Feed tab completion cache
		}
		m.appendLines(msg.Lines)
		return m, nil

	// General display line (server output or prompt commit)
	case DisplayLineMsg:
		m.wordCache.AddLine(string(msg))
		m.appendLines([]string{string(msg)})
		return m, nil

	// Single server line - batch for next tick
	case ServerLineMsg:
		m.pendingLines = append(m.pendingLines, string(msg))
		m.wordCache.AddLine(string(msg)) // Feed tab completion cache
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
		m.status.SetConnectionState(msg.State, msg.Address)
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
		if m.input.Value() == "" && m.input.Mode() == ModeNormal {
			m.quitting = true
			return m, tea.Quit
		}
		// Clear input or cancel overlay
		if m.input.Mode() != ModeNormal {
			m.input.SetMode(ModeNormal)
			m.slashPicker.Reset()
			m.fuzzySearch.Reset()
			return m, nil
		}
		m.input.Reset()
		return m, nil

	case tea.KeyEsc:
		if m.input.Mode() != ModeNormal {
			m.input.SetMode(ModeNormal)
			m.slashPicker.Reset()
			m.fuzzySearch.Reset()
			return m, nil
		}
		m.input.Reset()
		return m, nil
	}

	// Mode-specific handling
	switch m.input.Mode() {
	case ModeSlash:
		return m.handleSlashKey(msg)
	case ModeFuzzy:
		return m.handleFuzzyKey(msg)
	default:
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Check for bound pane toggle keys first (only when input is empty)
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
			m.input.AddToHistory(text)
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

	case tea.KeyCtrlR:
		m.input.SetMode(ModeFuzzy)
		m.fuzzySearch.SetHistory(m.input.History())
		m.fuzzySearch.Search("")
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
			m.input.SetMode(ModeSlash)
			// Load command lines from the "commands" pane
			m.slashPicker.SetLines(m.panes.GetLines("commands"))
			m.slashPicker.Filter("")
		}
	}

	// Forward to input
	newInput, cmd := m.input.Update(msg)
	m.input = *newInput

	// Update slash filter if in slash mode
	if m.input.Mode() == ModeSlash {
		m.slashPicker.Filter(m.input.GetSlashQuery())
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
			m.input.AddToHistory(text)
			select {
			case m.inputChan <- text:
			default:
			}
		} else if cmdName := m.slashPicker.SelectedCommand(); cmdName != "" {
			// No args - use selected command from picker
			fullCmd := "/" + cmdName
			m.input.AddToHistory(fullCmd)
			select {
			case m.inputChan <- fullCmd:
			default:
			}
		}
		m.input.Reset()
		m.input.SetMode(ModeNormal)
		m.slashPicker.Reset()
		return m, nil

	case tea.KeyTab:
		// Insert command name and position cursor at end
		if cmdName := m.slashPicker.SelectedCommand(); cmdName != "" {
			m.input.SetValue("/" + cmdName + " ")
			m.input.CursorEnd()
			m.input.ClearSuggestions()
		}
		m.input.SetMode(ModeNormal)
		m.slashPicker.Reset()
		return m, nil

	case tea.KeyBackspace:
		if m.input.Value() == "/" || m.input.Value() == "" {
			m.input.Reset()
			m.input.SetMode(ModeNormal)
			m.slashPicker.Reset()
			return m, nil
		}

	case tea.KeySpace:
		// Space commits to the selected command and exits picker mode
		// User can then type arguments freely
		if cmdName := m.slashPicker.SelectedCommand(); cmdName != "" {
			m.input.SetValue("/" + cmdName + " ")
			m.input.CursorEnd()
			m.input.ClearSuggestions()
		}
		m.input.SetMode(ModeNormal)
		m.slashPicker.Reset()
		return m, nil
	}

	// Forward to input and update filter
	newInput, cmd := m.input.Update(msg)
	m.input = *newInput
	m.slashPicker.Filter(m.input.GetSlashQuery())

	return m, cmd
}

func (m Model) handleFuzzyKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.fuzzySearch.SelectUp()
		return m, nil

	case tea.KeyDown:
		m.fuzzySearch.SelectDown()
		return m, nil

	case tea.KeyEnter, tea.KeyTab:
		// Insert selection
		if match := m.fuzzySearch.Selected(); match != nil {
			m.input.SetValue(match.Command)
		}
		m.input.SetMode(ModeNormal)
		m.fuzzySearch.Reset()
		return m, nil

	case tea.KeyCtrlR:
		// Toggle off
		m.input.SetMode(ModeNormal)
		m.fuzzySearch.Reset()
		return m, nil

	case tea.KeyRunes:
		// Add to search query
		m.fuzzySearch.Search(m.fuzzySearch.query + string(msg.Runes))
		return m, nil

	case tea.KeySpace:
		// Add space to search query
		m.fuzzySearch.Search(m.fuzzySearch.query + " ")
		return m, nil

	case tea.KeyBackspace:
		if len(m.fuzzySearch.query) > 0 {
			m.fuzzySearch.Search(m.fuzzySearch.query[:len(m.fuzzySearch.query)-1])
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
	m.status.SetScrollMode(m.viewport.Mode(), m.viewport.NewLineCount())
}

func (m *Model) updateDimensions() {
	// Reserve: 1 for infobar, 1 for separator, 1 for input, 1 for status
	reserved := 4

	// Account for visible panes at top
	paneHeight := m.panes.VisibleHeight()
	reserved += paneHeight

	viewportHeight := m.height - reserved
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	m.viewport.SetDimensions(m.width, viewportHeight)
	m.input.SetWidth(m.width)
	m.status.SetWidth(m.width)
	m.panes.SetWidth(m.width)
	m.slashPicker.SetWidth(m.width)
	m.fuzzySearch.SetWidth(m.width)
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.initialized {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	// Status bar (1 line)
	statusLine := m.status.View()

	// Input line (1 line)
	inputLine := m.input.View()

	// Info bar from Lua (1 line)
	infobarLine := m.infobar

	// Build the view
	var result string

	// Panes at top (if visible)
	if m.panes.HasVisiblePane() {
		result = m.panes.View() + "\n"
	}

	// Scrollback viewport
	result += m.viewport.View()

	// Overlay (if active) - rendered between scrollback and prompt
	switch m.input.Mode() {
	case ModeSlash:
		result += "\n" + m.slashPicker.View()
	case ModeFuzzy:
		result += "\n" + m.fuzzySearch.View()
	}

	// Add infobar, separator, input, status at bottom
	result += "\n" + infobarLine
	result += "\n\x1b[90m" + strings.Repeat("─", m.width) + "\x1b[0m"
	result += "\n" + inputLine
	result += "\n" + statusLine

	return result
}
