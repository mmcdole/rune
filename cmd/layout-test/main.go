// layout-test is a testbed for the UI layout system.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/drake/rune/ui"
	"github.com/drake/rune/ui/layout"
)

func main() {
	scenario := flag.String("scenario", "default", "Layout scenario (default, topbar, multi)")
	flag.Parse()

	provider := NewMockProvider()
	setupScenario(provider, *scenario)

	inputChan := make(chan string, 256)
	model := ui.NewModel(inputChan, provider)
	model.SetLayoutProvider(provider)

	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Handle input
	go func() {
		for input := range inputChan {
			if input == "/quit" || input == "quit" {
				program.Quit()
				return
			}
			program.Send(ui.EchoLineMsg("\033[32m> " + input + "\033[0m"))
		}
	}()

	// Send welcome message
	go func() {
		time.Sleep(100 * time.Millisecond)
		lines := []string{
			"\033[1;36mWelcome to the Layout Test!\033[0m",
			"",
			fmt.Sprintf("Current scenario: \033[33m%s\033[0m", *scenario),
			"",
			"Type /quit to exit",
			"Press / for slash command picker",
			"Press Ctrl+R for history search",
		}
		for _, line := range lines {
			program.Send(ui.DisplayLineMsg(line))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupScenario(p *MockProvider, scenario string) {
	// Register test commands
	p.commands = []ui.CommandInfo{
		{Name: "connect", Description: "Connect to a server"},
		{Name: "disconnect", Description: "Disconnect from server"},
		{Name: "quit", Description: "Exit the client"},
		{Name: "reload", Description: "Reload scripts"},
		{Name: "help", Description: "Show help"},
	}

	switch scenario {
	case "default":
		// Explicit default layout (input includes separator above it)
		p.layout = layout.Config{
			Bottom: []string{"input", "status"},
		}

	case "topbar":
		p.bars["title"] = &layout.BarDef{
			Name:   "title",
			Border: layout.BorderBottom,
			Render: func(state layout.ClientState, width int) layout.BarContent {
				return layout.BarContent{
					Left:  "\033[1;35m◆ RUNE MUD CLIENT ◆\033[0m",
					Right: time.Now().Format("15:04"),
				}
			},
		}
		p.layout = layout.Config{
			Top:    []string{"title"},
			Bottom: []string{"input", "status"},
		}

	case "multi":
		p.bars["title"] = &layout.BarDef{
			Name:   "title",
			Border: layout.BorderBottom,
			Render: func(state layout.ClientState, width int) layout.BarContent {
				return layout.BarContent{
					Left:  "\033[1;35m◆ MULTI-BAR LAYOUT ◆\033[0m",
					Right: time.Now().Format("2006-01-02"),
				}
			},
		}
		p.bars["stats"] = &layout.BarDef{
			Name: "stats",
			Render: func(state layout.ClientState, width int) layout.BarContent {
				return layout.BarContent{
					Left:   "\033[31mHP: 100/100\033[0m",
					Center: "\033[34mMP: 50/50\033[0m",
					Right:  "\033[33mGold: 1234\033[0m",
				}
			},
		}
		p.panes["combat"] = &layout.PaneDef{
			Name:    "combat",
			Height:  5,
			Visible: true,
			Title:   false,
			Border:  layout.BorderTop,
		}
		p.paneLines["combat"] = []string{
			"\033[31mYou hit the Orc for 25 damage!\033[0m",
			"\033[33mOrc hits you for 10 damage.\033[0m",
			"\033[31mYou hit the Orc for 30 damage!\033[0m",
			"\033[32mOrc is DEAD!\033[0m",
		}
		p.layout = layout.Config{
			Top:    []string{"title", "stats"},
			Bottom: []string{"combat", "input", "status"},
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown scenario: %s\n", scenario)
		fmt.Fprintln(os.Stderr, "Available: default, topbar, multi")
		os.Exit(1)
	}
}

// MockProvider implements layout.Provider and ui.DataProvider for testing.
type MockProvider struct {
	layout    layout.Config
	bars      map[string]*layout.BarDef
	panes     map[string]*layout.PaneDef
	paneLines map[string][]string
	state     layout.ClientState
	commands  []ui.CommandInfo
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		bars:      make(map[string]*layout.BarDef),
		panes:     make(map[string]*layout.PaneDef),
		paneLines: make(map[string][]string),
		state: layout.ClientState{
			Connected:  false,
			ScrollMode: "live",
		},
	}
}

// LayoutProvider implementation
func (m *MockProvider) Layout() layout.Config          { return m.layout }
func (m *MockProvider) Bar(name string) *layout.BarDef { return m.bars[name] }
func (m *MockProvider) Pane(name string) *layout.PaneDef { return m.panes[name] }
func (m *MockProvider) PaneLines(name string) []string { return m.paneLines[name] }
func (m *MockProvider) State() layout.ClientState      { return m.state }
func (m *MockProvider) RenderBars(width int) map[string]layout.BarContent { return nil }
func (m *MockProvider) HandleKeyBind(key string) bool { return false }

// DataProvider implementation
func (m *MockProvider) Commands() []ui.CommandInfo { return m.commands }
func (m *MockProvider) Aliases() []ui.AliasInfo    { return nil }
