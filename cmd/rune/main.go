package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/drake/rune/config"
	"github.com/drake/rune/engine"
	"github.com/drake/rune/internal/buffer"
	"github.com/drake/rune/mud"
	"github.com/drake/rune/network"
	"github.com/drake/rune/scripts"
	"github.com/drake/rune/ui"
)

func main() {
	// Parse flags
	simpleUI := flag.Bool("simple", false, "Use simple console UI instead of TUI")
	flag.Parse()

	// Event bus - unbounded buffer so producers never block or drop
	// Hard limit of 50,000 events prevents runaway memory usage
	eventsIn, eventsOut := buffer.Unbounded[mud.Event](100, 50000)

	// Network output channel
	netOut := make(chan string, 100)

	// Unbounded buffer for UI output
	// We use a hard limit of 50,000 lines. If the UI falls 50k lines behind,
	// the client is likely broken anyway, so we start dropping to save RAM.
	uiIn, uiOut := buffer.Unbounded[string](100, 50000)

	// UI control channel for Lua-driven status, panes, etc.
	uiControl := make(chan mud.UIControl, 100)

	// Component initialization
	luaEngine := engine.NewLuaEngine(eventsIn, netOut, uiIn, uiControl)
	tcpClient := network.NewTCPClient()

	// Select UI mode
	var tui mud.UI
	if *simpleUI {
		tui = ui.NewConsoleUI()
	} else {
		tui = ui.NewBubbleTeaUI()
	}

	defer luaEngine.Close()

	// Network -> Event loop
	go func() {
		for event := range tcpClient.Output() {
			eventsIn <- event
		}
	}()

	// UI -> Event loop
	go func() {
		for line := range tui.Input() {
			eventsIn <- mud.Event{Type: mud.EventUserInput, Payload: line}
		}
	}()

	// Network sender
	go func() {
		for line := range netOut {
			tcpClient.Send(line)
		}
	}()

	// UI renderer
	go func() {
		for line := range uiOut {
			tui.Render(line)
		}
	}()

	// UI control dispatcher (for Lua-driven status, panes)
	// Only works with BubbleTeaUI, silently ignored for ConsoleUI
	if btui, ok := tui.(*ui.BubbleTeaUI); ok {
		go func() {
			for ctrl := range uiControl {
				switch ctrl.Type {
				case mud.UIControlStatus:
					btui.SetStatusText(ctrl.Text)
				case mud.UIControlPaneCreate:
					btui.CreatePane(ctrl.Name)
				case mud.UIControlPaneWrite:
					btui.WritePane(ctrl.Name, ctrl.Text)
				case mud.UIControlPaneToggle:
					btui.TogglePane(ctrl.Name)
				case mud.UIControlPaneClear:
					btui.ClearPane(ctrl.Name)
				case mud.UIControlPaneBind:
					btui.BindPaneKey(ctrl.Key, ctrl.Name)
				case mud.UIControlInfobar:
					btui.SetInfobar(ctrl.Text)
				}
			}
		}()
	}

	// Initialize Lua state with core and user scripts
	if err := luaEngine.InitState(scripts.CoreScripts, config.Dir()); err != nil {
		uiIn <- fmt.Sprintf("\033[31m[Error] Loading scripts: %v\033[0m", err)
	}

	// Orchestrator loop - single goroutine owns Lua state
	go func() {
		for event := range eventsOut {
			switch event.Type {

			case mud.EventServerLine:
				modified, keep := luaEngine.OnOutput(event.Payload)
				if keep {
					// This write is now non-blocking.
					// Even if the UI freezes, this line is accepted instantly by the buffer.
					uiIn <- modified
				}

			case mud.EventServerPrompt:
				modified := luaEngine.OnPrompt(event.Payload)
				tui.RenderPrompt(modified)

			case mud.EventUserInput:
				tui.Render(event.Payload) // Local echo
				luaEngine.OnInput(event.Payload)

			case mud.EventTimer:
				if event.Callback != nil {
					luaEngine.ExecuteCallback(event.Callback)
				}

			case mud.EventSystemControl:
				switch event.Control.Action {
				case mud.ActionQuit:
					os.Exit(0)
				case mud.ActionConnect:
					addr := event.Control.Address
					luaEngine.CallHook("connecting", addr)
					go func() {
						if err := tcpClient.Connect(addr); err != nil {
							eventsIn <- mud.Event{
								Type: mud.EventTimer,
								Callback: func() {
									luaEngine.CallHook("error", "Connection failed: "+err.Error())
								},
							}
						} else {
							eventsIn <- mud.Event{
								Type: mud.EventTimer,
								Callback: func() {
									luaEngine.CallHook("connected", addr)
								},
							}
						}
					}()
				case mud.ActionDisconnect:
					luaEngine.CallHook("disconnecting")
					tcpClient.Disconnect()
					luaEngine.CallHook("disconnected")
				case mud.ActionReload:
					luaEngine.CallHook("reloading")
					if err := luaEngine.InitState(scripts.CoreScripts, config.Dir()); err != nil {
						luaEngine.CallHook("error", err.Error())
						break
					}
					luaEngine.CallHook("reloaded")
				case mud.ActionLoad:
					path := event.Control.ScriptPath
					if err := luaEngine.LoadUserScripts([]string{path}); err != nil {
						luaEngine.CallHook("error", err.Error())
					} else {
						luaEngine.CallHook("loaded", path)
					}
				}
			}
		}
	}()

	// Load user scripts from command line args (after flags)
	userScripts := flag.Args()
	if len(userScripts) > 0 {
		if err := luaEngine.LoadUserScripts(userScripts); err != nil {
			fmt.Println("Error loading user scripts:", err)
			os.Exit(1)
		}
	}

	// Block on UI
	if err := tui.Run(); err != nil {
		fmt.Println("UI error:", err)
		os.Exit(1)
	}
}
