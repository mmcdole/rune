package tui

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

// BubbleTeaUI implements interfaces.UI using Bubble Tea.
// It bridges the existing channel-based architecture with Bubble Tea's
// model/update/view event loop.
type BubbleTeaUI struct {
	program   *tea.Program
	inputChan chan input.Submission

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
		inputChan: make(chan input.Submission, 2048),
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

// Echo appends an already-styled local echo to scrollback. Styling is
// Lua policy (the "echo" hook); this method is transport only.
func (b *BubbleTeaUI) Echo(line string) {
	b.send(ui.EchoLineMsg(line))
}

// SetPrompt updates the active server prompt (overlay at bottom).
func (b *BubbleTeaUI) SetPrompt(text string) {
	b.send(ui.PromptMsg(text))
}

// Input returns channel for user input.
func (b *BubbleTeaUI) Input() <-chan input.Submission {
	return b.inputChan
}

// Run starts the TUI and blocks until exit.
func (b *BubbleTeaUI) Run() error {
	model := NewModel(b.inputChan, b.outbound)

	opts := []tea.ProgramOption{
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	}
	// On Windows, resize events only arrive through the console input
	// reader, which bubbletea engages only when the input is os.Stdin
	// itself. WithInputTTY opens CONIN$ as a separate handle, so bubbletea
	// silently falls back to its ANSI reader and window resizes (and
	// native mouse events) are never delivered.
	if runtime.GOOS != "windows" {
		opts = append(opts, tea.WithInputTTY())
	}
	b.program = tea.NewProgram(model, opts...)

	// Single goroutine drains message queue to Bubble Tea.
	// This can block on Send() without affecting producers.
	go func() {
		for {
			select {
			case <-b.done:
				return
			case msg := <-b.msgQueue:
				b.program.Send(msg)
			}
		}
	}()

	// Run blocks until quit
	_, err := b.program.Run()

	// Signal shutdown. The queue is deliberately never closed: send()
	// races the done signal in a select, and closing the channel would
	// turn a late Print from the session into a send-on-closed panic.
	b.doneOnce.Do(func() {
		close(b.done)
	})

	return err
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

// SetPaneVisible shows or hides a named pane.
func (b *BubbleTeaUI) SetPaneVisible(name string, visible bool) {
	b.send(ui.PaneSetVisibleMsg{Name: name, Visible: visible})
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
func (b *BubbleTeaUI) ShowPicker(opts ui.ShowPickerMsg) {
	b.send(opts)
}

// SetClipboard asks the terminal to set the system clipboard.
func (b *BubbleTeaUI) SetClipboard(text string) {
	b.send(ui.SetClipboardMsg(text))
}

// SetInput sets the input line content.
func (b *BubbleTeaUI) SetInput(text string) {
	b.send(ui.SetInputMsg(text))
}

// SetInputSubmission restores input text with an explicit interpretation.
func (b *BubbleTeaUI) SetInputSubmission(submission input.Submission) {
	b.send(ui.SetInputSubmissionMsg(submission))
}

// --- Input Primitives for Lua ---

// InputSetCursor sets the cursor position.
func (b *BubbleTeaUI) InputSetCursor(pos int) {
	b.send(ui.InputSetCursorMsg(pos))
}

// OpenEditor opens $EDITOR with the given initial text.
// Returns the edited content and whether the edit was successful.
func (b *BubbleTeaUI) OpenEditor(initial string) (string, bool) {
	// This is synchronous - we need to suspend the TUI
	if b.program == nil {
		return "", false
	}

	// Create temp file
	f, err := os.CreateTemp("", "rune-input-*.txt")
	if err != nil {
		return "", false
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if _, err := f.WriteString(initial); err != nil {
		_ = f.Close()
		return "", false
	}
	if err := f.Close(); err != nil {
		return "", false
	}

	// Suspend TUI
	if err := b.program.ReleaseTerminal(); err != nil {
		return "", false
	}

	// Run editor. The fallback must exist on the platform: vi ships
	// with effectively every Unix, notepad with every Windows.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()

	// Resume TUI
	restoreErr := b.program.RestoreTerminal()
	if restoreErr == nil {
		// Bubble Tea disables mouse reporting in ReleaseTerminal but does
		// not restore it in RestoreTerminal.
		b.program.Send(tea.EnableMouseCellMotion())
	}
	if err == nil && restoreErr != nil {
		err = restoreErr
	}

	if err != nil {
		return "", false
	}

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", false
	}
	return normalizeEditorText(string(content)), true
}

// normalizeEditorText converts platform newlines and removes exactly one
// trailing LF, which text editors conventionally use as the file terminator.
// All user-authored indentation, trailing spaces, tabs, and additional blank
// lines remain intact.
func normalizeEditorText(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.TrimSuffix(content, "\n")
}

// --- Pane Scrolling Primitives for Lua ---

// PaneScrollUp scrolls a pane up by N lines.
func (b *BubbleTeaUI) PaneScrollUp(name string, lines int) {
	b.send(ui.PaneScrollUpMsg{Name: name, Lines: lines})
}

// PaneScrollDown scrolls a pane down by N lines.
func (b *BubbleTeaUI) PaneScrollDown(name string, lines int) {
	b.send(ui.PaneScrollDownMsg{Name: name, Lines: lines})
}

// PaneScrollToTop scrolls a pane to the top.
func (b *BubbleTeaUI) PaneScrollToTop(name string) {
	b.send(ui.PaneScrollToTopMsg{Name: name})
}

// PaneScrollToBottom scrolls a pane to the bottom.
func (b *BubbleTeaUI) PaneScrollToBottom(name string) {
	b.send(ui.PaneScrollToBottomMsg{Name: name})
}

// --- Outbound messages from UI to Session ---

// Outbound returns a channel of messages from UI to Session.
// Session should read from this channel in its event loop.
func (b *BubbleTeaUI) Outbound() <-chan ui.UIEvent {
	return b.outbound
}
