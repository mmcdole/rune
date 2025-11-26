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

	// Synchronization for startup
	ready     chan struct{}
	readyOnce sync.Once

	// Shutdown coordination
	done     chan struct{}
	doneOnce sync.Once

	// Line batching
	pendingLines []string
	pendingMu    sync.Mutex

	// Pending messages queued before program starts
	pendingMsgs []tea.Msg
	pendingMsgMu sync.Mutex
}

// NewBubbleTeaUI creates a new Bubble Tea-based UI.
func NewBubbleTeaUI() *BubbleTeaUI {
	return &BubbleTeaUI{
		inputChan: make(chan string, 100),
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// sendOrQueue sends a message to the program, or queues it if not ready yet.
func (b *BubbleTeaUI) sendOrQueue(msg tea.Msg) {
	select {
	case <-b.ready:
		b.program.Send(msg)
	default:
		// Not ready yet, queue for later
		b.pendingMsgMu.Lock()
		b.pendingMsgs = append(b.pendingMsgs, msg)
		b.pendingMsgMu.Unlock()
	}
}

// Render implements mud.UI - queues a line for display.
// Called from the Orchestrator goroutine.
func (b *BubbleTeaUI) Render(text string) {
	// Wait for program to be ready
	<-b.ready

	// Send directly to Bubble Tea - let it handle batching via ticks
	b.program.Send(ServerLineMsg(text))
}

// flushLines sends all pending lines to the Bubble Tea model.
func (b *BubbleTeaUI) flushLines() {
	b.pendingMu.Lock()
	if len(b.pendingLines) == 0 {
		b.pendingMu.Unlock()
		return
	}
	lines := b.pendingLines
	b.pendingLines = nil
	b.pendingMu.Unlock()

	b.program.Send(flushLinesMsg{Lines: lines})
}

// RenderPrompt implements mud.UI - updates the prompt area.
// Called from the Orchestrator goroutine.
func (b *BubbleTeaUI) RenderPrompt(text string) {
	<-b.ready
	b.program.Send(PromptMsg(text))
}

// Input implements mud.UI - returns channel for user input.
func (b *BubbleTeaUI) Input() <-chan string {
	return b.inputChan
}

// Run implements mud.UI - starts the TUI and blocks until exit.
func (b *BubbleTeaUI) Run() error {
	model := NewModel(b.inputChan)

	b.program = tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithInputTTY(),
	)

	// Flush pending messages in a goroutine after a short delay
	// to ensure the program event loop has started
	go func() {
		b.pendingMsgMu.Lock()
		msgs := b.pendingMsgs
		b.pendingMsgs = nil
		b.pendingMsgMu.Unlock()

		for _, msg := range msgs {
			b.program.Send(msg)
		}
	}()

	// Signal that program is ready for messages
	b.readyOnce.Do(func() {
		close(b.ready)
	})

	// Run blocks until quit
	_, err := b.program.Run()

	// Signal shutdown
	b.doneOnce.Do(func() {
		close(b.done)
	})

	return err
}

// Done returns a channel that closes when the UI exits.
func (b *BubbleTeaUI) Done() <-chan struct{} {
	return b.done
}

// Quit signals the TUI to exit.
func (b *BubbleTeaUI) Quit() {
	select {
	case <-b.ready:
		if b.program != nil {
			b.program.Quit()
		}
	default:
		// Not started yet, just close done
		b.doneOnce.Do(func() {
			close(b.done)
		})
	}
}

// SetConnectionState updates the connection status display.
func (b *BubbleTeaUI) SetConnectionState(state ConnectionState, addr string) {
	select {
	case <-b.ready:
		b.program.Send(ConnectionStateMsg{State: state, Address: addr})
	default:
		// Not ready yet, ignore
	}
}

// SetStatus sets the status bar text (called from Lua via rune.status.set).
func (b *BubbleTeaUI) SetStatus(text string) {
	select {
	case <-b.ready:
		b.program.Send(StatusTextMsg(text))
	default:
		// Not ready yet, ignore
	}
}

// SetInfobar sets the info bar text (called from Lua via rune.infobar.set).
func (b *BubbleTeaUI) SetInfobar(text string) {
	select {
	case <-b.ready:
		b.program.Send(InfobarMsg(text))
	default:
		// Not ready yet, ignore
	}
}

// CreatePane creates a new named pane.
func (b *BubbleTeaUI) CreatePane(name string) {
	b.sendOrQueue(PaneCreateMsg{Name: name})
}

// WritePane writes a line to a named pane.
func (b *BubbleTeaUI) WritePane(name, text string) {
	b.sendOrQueue(PaneWriteMsg{Name: name, Text: text})
}

// TogglePane toggles visibility of a named pane.
func (b *BubbleTeaUI) TogglePane(name string) {
	b.sendOrQueue(PaneToggleMsg{Name: name})
}

// ClearPane clears the contents of a named pane.
func (b *BubbleTeaUI) ClearPane(name string) {
	b.sendOrQueue(PaneClearMsg{Name: name})
}

// BindPaneKey binds a key to toggle a pane.
func (b *BubbleTeaUI) BindPaneKey(key, name string) {
	b.sendOrQueue(PaneBindMsg{Key: key, Name: name})
}
