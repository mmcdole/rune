package ui

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

// ConsoleUI implements a simple stdin/stdout UI
type ConsoleUI struct {
	inputChan     chan string
	done          chan struct{}
	doneOnce      sync.Once
	lastWasPrompt bool   // Track if last output was a prompt (no newline)
	lastPrompt    string // Track last prompt content for deduplication
}

// NewConsoleUI initializes a stdin/stdout based terminal interface.
func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{
		inputChan: make(chan string, 2048),
		done:      make(chan struct{}),
	}
}

// Render outputs text to the console (with newline)
func (c *ConsoleUI) Render(text string) {
	if c.lastWasPrompt {
		// Clear the prompt line before printing new content
		fmt.Print("\r\033[K")
		c.lastWasPrompt = false
		c.lastPrompt = ""
	}
	fmt.Println(text)
}

// RenderDisplayLine outputs text to the console (alias of Render)
func (c *ConsoleUI) RenderDisplayLine(text string) {
	c.Render(text)
}

// RenderEcho outputs a local echo line with a prompt-style prefix
func (c *ConsoleUI) RenderEcho(text string) {
	c.Render("\033[32m> " + text + "\033[0m")
}

// RenderPrompt outputs a prompt without a trailing newline
// Subsequent calls will overwrite the previous prompt
func (c *ConsoleUI) RenderPrompt(text string) {
	// Skip if same prompt (avoid clearing user's typing)
	if c.lastWasPrompt && text == c.lastPrompt {
		return
	}

	if c.lastWasPrompt {
		// Clear previous prompt
		fmt.Print("\r\033[K")
	}
	fmt.Print(text)
	c.lastWasPrompt = true
	c.lastPrompt = text
}

// Input returns the channel for receiving user input
func (c *ConsoleUI) Input() <-chan string {
	return c.inputChan
}

// Run starts the UI and blocks until done
func (c *ConsoleUI) Run() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanDone := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			select {
			case <-c.done:
				scanDone <- nil
				return
			default:
			}
			text := scanner.Text()
			c.inputChan <- text
		}
		scanDone <- scanner.Err()
	}()

	select {
	case <-c.done:
		return nil
	case err := <-scanDone:
		return err
	}
}

// Done returns a channel that closes when the UI is done
func (c *ConsoleUI) Done() <-chan struct{} {
	return c.done
}

// Quit requests the console UI to exit.
func (c *ConsoleUI) Quit() {
	c.doneOnce.Do(func() {
		close(c.done)
	})
}

// Controller methods (no-op for ConsoleUI - advanced features not supported in simple mode)

func (c *ConsoleUI) SetStatus(text string)        {}
func (c *ConsoleUI) SetInfobar(text string)       {}
func (c *ConsoleUI) CreatePane(name string)       {}
func (c *ConsoleUI) WritePane(name, text string)  {}
func (c *ConsoleUI) TogglePane(name string)       {}
func (c *ConsoleUI) ClearPane(name string)        {}
func (c *ConsoleUI) BindPaneKey(key, name string) {}
