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

	inputChan := make(chan string, 256)
	outboundChan := make(chan any, 256)
	model := ui.NewModel(inputChan, outboundChan)

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

	// Drain outbound messages and handle window size for bar updates
	go func() {
		width := 80 // Default width
		for msg := range outboundChan {
			if wsm, ok := msg.(ui.WindowSizeChangedMsg); ok {
				width = wsm.Width
				// Re-render bars with new width
				bars := renderBars(*scenario, width)
				program.Send(ui.UpdateBarsMsg(bars))
			}
		}
	}()

	// Push initial layout and bars after a short delay (after UI initializes)
	go func() {
		time.Sleep(50 * time.Millisecond)

		// Push layout configuration
		layoutCfg := getLayout(*scenario)
		program.Send(ui.UpdateLayoutMsg{Top: layoutCfg.Top, Bottom: layoutCfg.Bottom})

		// Push initial bar content (with default width until resize)
		bars := renderBars(*scenario, 80)
		program.Send(ui.UpdateBarsMsg(bars))

		// Send welcome message
		time.Sleep(50 * time.Millisecond)
		lines := []string{
			"\033[1;36mWelcome to the Layout Test!\033[0m",
			"",
			fmt.Sprintf("Current scenario: \033[33m%s\033[0m", *scenario),
			"",
			"Type /quit to exit",
		}
		for _, line := range lines {
			program.Send(ui.DisplayLineMsg(line))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Periodic bar updates (simulates Session's bar ticker)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		width := 80
		for range ticker.C {
			bars := renderBars(*scenario, width)
			program.Send(ui.UpdateBarsMsg(bars))
		}
	}()

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getLayout(scenario string) layout.Config {
	switch scenario {
	case "default":
		return layout.Config{
			Bottom: []string{"input", "status"},
		}
	case "topbar":
		return layout.Config{
			Top:    []string{"title"},
			Bottom: []string{"input", "status"},
		}
	case "multi":
		return layout.Config{
			Top:    []string{"title", "stats"},
			Bottom: []string{"input", "status"},
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown scenario: %s\n", scenario)
		fmt.Fprintln(os.Stderr, "Available: default, topbar, multi")
		os.Exit(1)
		return layout.Config{}
	}
}

func renderBars(scenario string, width int) map[string]layout.BarContent {
	bars := make(map[string]layout.BarContent)

	switch scenario {
	case "topbar":
		bars["title"] = layout.BarContent{
			Left:  "\033[1;35m◆ RUNE MUD CLIENT ◆\033[0m",
			Right: time.Now().Format("15:04:05"),
		}
	case "multi":
		bars["title"] = layout.BarContent{
			Left:  "\033[1;35m◆ MULTI-BAR LAYOUT ◆\033[0m",
			Right: time.Now().Format("2006-01-02 15:04:05"),
		}
		bars["stats"] = layout.BarContent{
			Left:   "\033[31mHP: 100/100\033[0m",
			Center: "\033[34mMP: 50/50\033[0m",
			Right:  "\033[33mGold: 1234\033[0m",
		}
	}

	return bars
}
