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

	// Shutdown coordination
	done     chan struct{}
	doneOnce sync.Once

	// Data provider for UI overlays (set before Run)
	provider DataProvider
}

// NewBubbleTeaUI creates a new Bubble Tea-based UI.
func NewBubbleTeaUI() *BubbleTeaUI {
	return &BubbleTeaUI{
		inputChan: make(chan string, 2048),
		msgQueue:  make(chan tea.Msg, 4096),
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

// SetDataProvider sets the data provider for commands and aliases.
// Must be called before Run().
func (b *BubbleTeaUI) SetDataProvider(p DataProvider) {
	b.provider = p
}

// Run implements mud.UI - starts the TUI and blocks until exit.
func (b *BubbleTeaUI) Run() error {
	model := NewModel(b.inputChan, b.provider)

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
