package ui

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// BubbleTeaUI implements mud.UI using Bubble Tea.
// It bridges the existing channel-based architecture with Bubble Tea's
// model/update/view event loop.
type BubbleTeaUI struct {
	program   *tea.Program
	inputChan chan string

	// Message queue - buffered channel drained by a single goroutine.
	// This decouples callers from tea.Program.Send() which can block.
	msgQueue chan tea.Msg

	// Outbound messages from UI to Session (e.g., ExecuteBindMsg, WindowSizeChangedMsg)
	// Session reads from this channel in its event loop.
	outbound chan any

	// Shutdown coordination
	done     chan struct{}
	doneOnce sync.Once
}

// NewBubbleTeaUI creates a new Bubble Tea-based UI.
func NewBubbleTeaUI() *BubbleTeaUI {
	return &BubbleTeaUI{
		inputChan: make(chan string, 2048),
		msgQueue:  make(chan tea.Msg, 4096),
		outbound:  make(chan any, 256),
		done:      make(chan struct{}),
	}
}

// send queues a message for delivery to the Bubble Tea program.
// Never blocks - drops message if queue is full.
func (b *BubbleTeaUI) send(msg tea.Msg) {
	select {
	case <-b.done:
		return
	case b.msgQueue <- msg:
	default:
		// Drop rather than block producers
	}
}

// Render implements mud.UI - queues a line for display.
func (b *BubbleTeaUI) Render(text string) {
	b.send(ServerLineMsg(text))
}

// RenderDisplayLine implements mud.UI - queues a display line for scrollback.
func (b *BubbleTeaUI) RenderDisplayLine(text string) {
	b.send(DisplayLineMsg(text))
}

// RenderEcho implements mud.UI - queues a local echo line.
func (b *BubbleTeaUI) RenderEcho(text string) {
	styled := "\033[32m> " + text + "\033[0m"
	b.send(EchoLineMsg(styled))
}

// RenderPrompt implements mud.UI - updates the prompt area.
func (b *BubbleTeaUI) RenderPrompt(text string) {
	b.send(PromptMsg(text))
}

// Input implements mud.UI - returns channel for user input.
func (b *BubbleTeaUI) Input() <-chan string {
	return b.inputChan
}

// Run implements mud.UI - starts the TUI and blocks until exit.
func (b *BubbleTeaUI) Run() error {
	model := NewModel(b.inputChan, b.outbound)

	b.program = tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithInputTTY(),
	)

	// Single goroutine drains message queue to Bubble Tea.
	// This can block on Send() without affecting producers.
	go func() {
		for {
			select {
			case <-b.done:
				return
			case msg, ok := <-b.msgQueue:
				if !ok {
					return
				}
				b.program.Send(msg)
			}
		}
	}()

	// Run blocks until quit
	_, err := b.program.Run()

	// Signal shutdown and close queue
	b.doneOnce.Do(func() {
		close(b.done)
	})
	close(b.msgQueue)

	return err
}

// Done returns a channel that closes when the UI exits.
func (b *BubbleTeaUI) Done() <-chan struct{} {
	return b.done
}

// Quit signals the TUI to exit.
func (b *BubbleTeaUI) Quit() {
	if b.program != nil {
		b.program.Quit()
	}
	b.doneOnce.Do(func() {
		close(b.done)
	})
}

// SetConnectionState updates the connection status display.
func (b *BubbleTeaUI) SetConnectionState(state ConnectionState, addr string) {
	b.send(ConnectionStateMsg{State: state, Address: addr})
}

// SetStatus sets the status bar text (called from Lua via rune.status.set).
func (b *BubbleTeaUI) SetStatus(text string) {
	b.send(StatusTextMsg(text))
}

// SetInfobar sets the info bar text (called from Lua via rune.infobar.set).
func (b *BubbleTeaUI) SetInfobar(text string) {
	b.send(InfobarMsg(text))
}

// CreatePane creates a new named pane.
func (b *BubbleTeaUI) CreatePane(name string) {
	b.send(PaneCreateMsg{Name: name})
}

// WritePane writes a line to a named pane.
func (b *BubbleTeaUI) WritePane(name, text string) {
	b.send(PaneWriteMsg{Name: name, Text: text})
}

// TogglePane toggles visibility of a named pane.
func (b *BubbleTeaUI) TogglePane(name string) {
	b.send(PaneToggleMsg{Name: name})
}

// ClearPane clears the contents of a named pane.
func (b *BubbleTeaUI) ClearPane(name string) {
	b.send(PaneClearMsg{Name: name})
}

// BindPaneKey binds a key to toggle a pane.
func (b *BubbleTeaUI) BindPaneKey(key, name string) {
	b.send(PaneBindMsg{Key: key, Name: name})
}

// --- Push-based messages from Session to UI ---

// UpdateBars sends rendered bar content from Session to UI.
func (b *BubbleTeaUI) UpdateBars(content map[string]BarContent) {
	b.send(UpdateBarsMsg(content))
}

// UpdateBinds sends the current set of bound keys from Session to UI.
func (b *BubbleTeaUI) UpdateBinds(keys map[string]bool) {
	b.send(UpdateBindsMsg(keys))
}

// UpdateLayout sends layout configuration from Session to UI.
func (b *BubbleTeaUI) UpdateLayout(top, bottom []string) {
	b.send(UpdateLayoutMsg{Top: top, Bottom: bottom})
}

// UpdateHistory sends input history from Session to UI.
func (b *BubbleTeaUI) UpdateHistory(history []string) {
	b.send(UpdateHistoryMsg(history))
}

// ShowPicker displays a picker overlay with items.
// prefix enables inline mode: picker filters based on input line minus prefix.
func (b *BubbleTeaUI) ShowPicker(title string, items []GenericItem, callbackID string, prefix string) {
	b.send(ShowPickerMsg{Title: title, Items: items, CallbackID: callbackID, Prefix: prefix})
}

// SetInput sets the input line content.
func (b *BubbleTeaUI) SetInput(text string) {
	b.send(SetInputMsg(text))
}

// --- Outbound messages from UI to Session ---

// Outbound returns a channel of messages from UI to Session.
// Session should read from this channel in its event loop.
func (b *BubbleTeaUI) Outbound() <-chan any {
	return b.outbound
}
