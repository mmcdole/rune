package main

import (
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
	// Event bus and coordination channels
	events := make(chan mud.Event, 100)
	netOut := make(chan string, 100)

	// Unbounded buffer for UI output
	// We use a hard limit of 50,000 lines. If the UI falls 50k lines behind,
	// the client is likely broken anyway, so we start dropping to save RAM.
	uiIn, uiOut := buffer.Unbounded[string](100, 50000)

	// Component initialization
	luaEngine := engine.NewLuaEngine(events, netOut, uiIn)
	tcpClient := network.NewTCPClient()
	tui := ui.NewConsoleUI()

	defer luaEngine.Close()

	// Network -> Event loop
	go func() {
		for event := range tcpClient.Output() {
			events <- event
		}
	}()

	// UI -> Event loop (Non-blocking to prevent deadlock)
	go func() {
		for line := range tui.Input() {
			evt := mud.Event{Type: mud.EventUserInput, Payload: line}
			select {
			case events <- evt:
				// Success: Event queued normally
			default:
				// Channel full - give feedback instead of blocking
				tui.Render("\033[31m[WARNING] Engine lagging. Input dropped: " + line + "\033[0m")
			}
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

	// Initialize Lua state with core and user scripts
	if err := luaEngine.InitState(scripts.CoreScripts, config.Dir()); err != nil {
		uiIn <- fmt.Sprintf("\033[31m[Error] Loading scripts: %v\033[0m", err)
	}

	// Orchestrator loop - single goroutine owns Lua state
	go func() {
		for event := range events {
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
				tui.Render("> " + event.Payload) // Local echo
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
							events <- mud.Event{
								Type: mud.EventTimer,
								Callback: func() {
									luaEngine.CallHook("error", "Connection failed: "+err.Error())
								},
							}
						} else {
							events <- mud.Event{
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

	// Load user scripts from command line args
	userScripts := os.Args[1:]
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
