package tui

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/drake/rune/ui"
)

// BubbleTeaUI implements interfaces.UI using Bubble Tea.
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
	outbound chan ui.UIEvent

	// Shutdown coordination
	done     chan struct{}
	doneOnce sync.Once
}

// NewBubbleTeaUI creates a new Bubble Tea-based UI.
func NewBubbleTeaUI() *BubbleTeaUI {
	return &BubbleTeaUI{
		inputChan: make(chan string, 2048),
		msgQueue:  make(chan tea.Msg, 4096),
		outbound:  make(chan ui.UIEvent, 256),
		done:      make(chan struct{}),
	}
}

// send queues a message for delivery to the Bubble Tea program.
// Blocks until message is queued - never drops messages.
// For a MUD client, dropping server output is unacceptable.
func (b *BubbleTeaUI) send(msg tea.Msg) {
	select {
	case <-b.done:
		return
	case b.msgQueue <- msg:
	}
}

// Print appends text to the main scrollback buffer.
// All output (server lines, Lua prints) goes through this single method.
func (b *BubbleTeaUI) Print(text string) {
	b.send(ui.PrintLineMsg(text))
}

// Echo appends user input to scrollback with local-echo styling.
func (b *BubbleTeaUI) Echo(text string) {
	styled := "\033[32m> " + text + "\033[0m"
	b.send(ui.EchoLineMsg(styled))
}

// SetPrompt updates the active server prompt (overlay at bottom).
func (b *BubbleTeaUI) SetPrompt(text string) {
	b.send(ui.PromptMsg(text))
}

// Input returns channel for user input.
func (b *BubbleTeaUI) Input() <-chan string {
	return b.inputChan
}

// Run starts the TUI and blocks until exit.
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
func (b *BubbleTeaUI) SetConnectionState(state ui.ConnectionState, addr string) {
	b.send(ui.ConnectionStateMsg{State: state, Address: addr})
}

// CreatePane creates a new named pane.
func (b *BubbleTeaUI) CreatePane(name string) {
	b.send(ui.PaneCreateMsg{Name: name})
}

// WritePane writes a line to a named pane.
func (b *BubbleTeaUI) WritePane(name, text string) {
	b.send(ui.PaneWriteMsg{Name: name, Text: text})
}

// TogglePane toggles visibility of a named pane.
func (b *BubbleTeaUI) TogglePane(name string) {
	b.send(ui.PaneToggleMsg{Name: name})
}

// ClearPane clears the contents of a named pane.
func (b *BubbleTeaUI) ClearPane(name string) {
	b.send(ui.PaneClearMsg{Name: name})
}

// --- Push-based messages from Session to UI ---

// UpdateBars sends rendered bar content from Session to UI.
func (b *BubbleTeaUI) UpdateBars(content map[string]ui.BarContent) {
	b.send(ui.UpdateBarsMsg(content))
}

// UpdateBinds sends the current set of bound keys from Session to UI.
func (b *BubbleTeaUI) UpdateBinds(keys map[string]bool) {
	b.send(ui.UpdateBindsMsg(keys))
}

// UpdateLayout sends layout configuration from Session to UI.
func (b *BubbleTeaUI) UpdateLayout(top, bottom []ui.LayoutEntry) {
	b.send(ui.UpdateLayoutMsg{Top: top, Bottom: bottom})
}

// ShowPicker displays a picker overlay with items.
// inline: if true, picker filters based on input; if false, picker captures keyboard.
func (b *BubbleTeaUI) ShowPicker(title string, items []ui.PickerItem, callbackID string, inline bool) {
	b.send(ui.ShowPickerMsg{Title: title, Items: items, CallbackID: callbackID, Inline: inline})
}

// SetInput sets the input line content.
func (b *BubbleTeaUI) SetInput(text string) {
	b.send(ui.SetInputMsg(text))
}

// --- Outbound messages from UI to Session ---

// Outbound returns a channel of messages from UI to Session.
// Session should read from this channel in its event loop.
func (b *BubbleTeaUI) Outbound() <-chan ui.UIEvent {
	return b.outbound
}
