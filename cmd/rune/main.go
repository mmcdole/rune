package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drake/rune/engine"
	"github.com/drake/rune/mud"
	"github.com/drake/rune/network"
	"github.com/drake/rune/scripts"
	"github.com/drake/rune/ui"
)

func main() {
	// Event bus and coordination channels
	events := make(chan mud.Event, 100)
	netOut := make(chan string, 100)
	uiOut := make(chan string, 100)

	// Component initialization
	luaEngine := engine.NewLuaEngine()
	tcpClient := network.NewTCPClient()
	tui := ui.NewConsoleUI()

	// Setup engine bindings
	luaEngine.RegisterHostFuncs(events, netOut, uiOut)

	// Set config directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		configDir := filepath.Join(homeDir, ".config", "rune")
		luaEngine.SetConfigDir(configDir)
	}

	// Load embedded core scripts
	fmt.Println("Loading core scripts...")
	if err := luaEngine.LoadEmbeddedCore(scripts.CoreScripts); err != nil {
		fmt.Println("Error loading core scripts:", err)
		os.Exit(1)
	}
	fmt.Println("Core scripts loaded.")

	// Auto-load ~/.config/rune/init.lua if it exists
	if homeDir, err := os.UserHomeDir(); err == nil {
		initPath := filepath.Join(homeDir, ".config", "rune", "init.lua")
		if _, err := os.Stat(initPath); err == nil {
			fmt.Println("Loading init.lua...")
			if err := luaEngine.LoadUserScripts([]string{initPath}); err != nil {
				fmt.Println("Error loading init.lua:", err)
				// Continue anyway - don't exit on user script error
			}
			fmt.Println("init.lua loaded.")
		}
	}

	// Load user scripts from command line args
	userScripts := os.Args[1:]
	if len(userScripts) > 0 {
		if err := luaEngine.LoadUserScripts(userScripts); err != nil {
			fmt.Println("Error loading user scripts:", err)
			os.Exit(1)
		}
	}

	defer luaEngine.Close()

	// Network -> Event loop
	go func() {
		for event := range tcpClient.Output() {
			events <- event
		}
	}()

	// UI -> Event loop
	go func() {
		for line := range tui.Input() {
			events <- mud.Event{Type: mud.EventUserInput, Payload: line}
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

	// Orchestrator loop - single goroutine owns Lua state
	go func() {
		for event := range events {
			switch event.Type {

			case mud.EventServerLine:
				modified, keep := luaEngine.OnOutput(event.Payload)
				if keep {
					uiOut <- modified
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
					luaEngine.CallHook("on_sys_connecting", addr)
					go func() {
						if err := tcpClient.Connect(addr); err != nil {
							events <- mud.Event{
								Type: mud.EventTimer,
								Callback: func() {
									luaEngine.CallHook("on_sys_error", "Connection failed: "+err.Error())
								},
							}
						} else {
							events <- mud.Event{
								Type: mud.EventTimer,
								Callback: func() {
									luaEngine.CallHook("on_sys_connected", addr)
								},
							}
						}
					}()
				case mud.ActionDisconnect:
					luaEngine.CallHook("on_sys_disconnecting")
					tcpClient.Disconnect()
					luaEngine.CallHook("on_sys_disconnected")
				case mud.ActionReload:
					luaEngine.CallHook("on_sys_reloading")
					if homeDir, err := os.UserHomeDir(); err == nil {
						configDir := filepath.Join(homeDir, ".config", "rune")
						if err := luaEngine.Reload(scripts.CoreScripts, configDir); err != nil {
							luaEngine.CallHook("on_sys_error", err.Error())
							break
						}
					}
					luaEngine.CallHook("on_sys_reloaded")
				case mud.ActionLoad:
					path := event.Control.ScriptPath
					if err := luaEngine.LoadUserScripts([]string{path}); err != nil {
						luaEngine.CallHook("on_sys_error", err.Error())
					} else {
						luaEngine.CallHook("on_sys_loaded", path)
					}
				}
			}
		}
	}()

	// Block on UI
	if err := tui.Run(); err != nil {
		fmt.Println("UI error:", err)
		os.Exit(1)
	}
}
