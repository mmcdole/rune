
This is a clean-slate design. We will use the Actor Model architecture.
In this design, there is one Goroutine (the "Orchestrator") that owns the Lua state. It never sleeps and never blocks. All other components (Network, UI, Timers) talk to it via Channels.
This eliminates the need for Mutexes, prevents deadlocks, guarantees thread safety, and handles the "Wait" problem elegantly.
1. System Architecture
code
Text
[UI Input]           [Network Incoming]      [Timer System]
           |                        |                     |
           v                        v                     v
      (InputChan)            (ServerChan)            (TimerChan)
           \                        |                        /
            \_______________________|_______________________/
                                    |
                          [ THE ORCHESTRATOR ]
                    (Owns Lua State & Event Loop)
                                    |
               _____________________|_____________________
               |                    |                    |
    [Lua: Process Input]   [Lua: Process Output]   [Lua: Run Callback]
               |                    |
               v                    v
          (SendChan)           (RenderChan)
               |                    |
               v                    v
       [Network Writer]        [UI Renderer]
2. Core Interfaces
We define the contracts for the outside world.
code
Go
package mud

import "time"

// EventType identifies the source of the message
type EventType int

const (
	EventUserInput EventType = iota
	EventServerData
	EventTimer
)

// Event is the universal packet sent to the Orchestrator
type Event struct {
	Type    EventType
	Payload string          // For User/Server text
	Callback func()         // For Timers (Lua Closures)
}

// ScriptEngine describes the Lua interactions
type ScriptEngine interface {
	// Init loads the Lua state and scripts
	Init(corePath, userPath string) error
	
	// RegisterHostFuncs allows Go to bind 'client.send', 'client.timer'
	// The 'out' channel is how the Engine sends commands back to the Orchestrator
	RegisterHostFuncs(out chan<- Event, sendToNet chan<- string, renderToUI chan<- string)

	// OnInput handles user typing. Returns true if swallowed/handled.
	OnInput(text string) bool

	// OnOutput handles server text. Returns modified text and boolean (false = gag).
	OnOutput(text string) (string, bool)
    
    // ExecuteCallback runs a stored Lua function (from a timer)
    ExecuteCallback(cb func())
    
    // Close cleans up
    Close()
}

// Network defines the TCP/Telnet layer
type Network interface {
	Connect(address string) error
	Disconnect()
	Send(data string)       // Non-blocking write
	Output() <-chan string  // Stream from server
}

// UI defines the Terminal layer
type UI interface {
	Render(text string)
	Input() <-chan string   // Stream from user
	Run() error
}
3. The Implementation: The Orchestrator
This is the most important part. It unifies the system.
code
Go
package main

import (
	"time"
	// imports for your implementation packages...
)

func main() {
	// 1. Create Channels (The Nervous System)
	// Buffered to prevent temporary blocking
	eventChan := make(chan mud.Event, 100)
	netSendChan := make(chan string, 100)
	uiRenderChan := make(chan string, 100)

	// 2. Initialize Components
	engine := NewLuaEngine() // Implementation of ScriptEngine
	net := NewTelnetClient()
	gui := NewTUI()

	// 3. Setup Engine Bindings
	// This allows Lua to inject events back into the main loop
	engine.RegisterHostFuncs(eventChan, netSendChan, uiRenderChan)
	engine.Init("./core.lua", "./user.lua")

	// 4. Connect Network
	net.Connect("mud.example.com:4000")

	// 5. Start Input Feeders (Producers)
	
	// Network -> EventLoop
	go func() {
		for line := range net.Output() {
			eventChan <- mud.Event{Type: mud.EventServerData, Payload: line}
		}
	}()

	// UI -> EventLoop
	go func() {
		for line := range gui.Input() {
			eventChan <- mud.Event{Type: mud.EventUserInput, Payload: line}
		}
	}()

	// NetSender Consumer
	go func() {
		for line := range netSendChan {
			net.Send(line)
		}
	}()

	// UI Renderer Consumer
	go func() {
		for line := range uiRenderChan {
			gui.Render(line)
		}
	}()

	// 6. THE ORCHESTRATOR LOOP (The Actor)
	// This runs in a separate goroutine (or main thread).
	// It is the ONLY thing that touches the Lua State.
	go func() {
		for event := range eventChan {
			switch event.Type {
			
			case mud.EventServerData:
				// Server -> Lua -> UI
				modified, keep := engine.OnOutput(event.Payload)
				if keep {
					uiRenderChan <- modified
				}

			case mud.EventUserInput:
				// User -> Lua -> Network
				// Note: We don't send directly here. Lua decides via client.send()
				// This handles aliases eating commands.
				gui.Render("> " + event.Payload) // Local echo
				engine.OnInput(event.Payload)

			case mud.EventTimer:
				// Timer -> Lua
				if event.Callback != nil {
					engine.ExecuteCallback(event.Callback)
				}
			}
		}
	}()

	// 7. Block on UI
	gui.Run()
}
4. The Lua Engine Implementation
Here is how we bridge Go and Lua without mutexes, using the channels provided during registration.
code
Go
type LuaEngine struct {
	L *lua.LState
}

func (e *LuaEngine) RegisterHostFuncs(loopChan chan<- mud.Event, netChan chan<- string, uiChan chan<- string) {
	
	// 1. client.send("kill orc")
	e.L.SetGlobal("client_send", e.L.NewFunction(func(L *lua.LState) int {
		cmd := L.CheckString(1)
		netChan <- cmd // Fire and forget
		return 0
	}))

	// 2. client.print("local message")
	e.L.SetGlobal("client_print", e.L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		uiChan <- msg
		return 0
	}))

	// 3. client.delay(2.5, callback_func)
	e.L.SetGlobal("client_delay", e.L.NewFunction(func(L *lua.LState) int {
		seconds := L.CheckNumber(1)
		fn := L.CheckFunction(2) // The Lua Closure

		// We verify the function, but we can't run it yet.
		// We start a Go timer.
		time.AfterFunc(time.Duration(seconds*float64(time.Second)), func() {
			// When timer fires, we DO NOT touch Lua.
			// We send an event back to the Orchestrator.
			loopChan <- mud.Event{
				Type: mud.EventTimer,
				Callback: func() {
					// This closure captures 'fn' and 'L'
					// It will be run by the Orchestrator later.
					L.Push(fn)
					L.PCall(0, 0, nil)
				},
			}
		})
		return 0
	}))
}
5. The Lua Logic (Core Scripts)
This handles the requirement for ; delimiters, recursion, and #wait.
core.lua
code
Lua
-- Core Configuration
local DELIMITER = ";"
local WAIT_CMD = "#wait"

-- Aliases storage
local aliases = {}

function add_alias(key, value)
    aliases[key] = value
end

-- Helper: Split string into a queue
local function parse_to_queue(input)
    local queue = {}
    for part in string.gmatch(input, "[^"..DELIMITER.."]+") do
        table.insert(queue, part:match("^%s*(.-)%s*$"))
    end
    return queue
end

-- The Recursive Executor
local function process_queue(queue)
    if #queue == 0 then return end

    -- Pop first item
    local current = table.remove(queue, 1)
    if current == "" then return process_queue(queue) end

    -- 1. Check for #wait
    local wait_time = current:match("^"..WAIT_CMD.."%s+(%d+)")
    if wait_time then
        -- Call Go API: delay(seconds, callback)
        -- The callback recursively processes the REST of the queue
        client_print("[System] Pausing for " .. wait_time .. "s...")
        client_delay(tonumber(wait_time), function()
            process_queue(queue)
        end)
        return -- Stop processing this batch, the callback handles the rest
    end

    -- 2. Check for Alias
    if aliases[current] then
        -- Expand alias
        local expansion = aliases[current]
        -- Parse expansion into a new queue
        local expanded_queue = parse_to_queue(expansion)
        
        -- Prepend expanded items to the FRONT of the current queue
        -- to maintain order: s;7w -> s;w;w;w...
        for i = #expanded_queue, 1, -1 do
            table.insert(queue, 1, expanded_queue[i])
        end
        
        -- Recurse immediately
        return process_queue(queue)
    end

    -- 3. Normal Command
    client_send(current)
    
    -- 4. Continue with rest of queue
    process_queue(queue)
end

-- Hook called by Go
function on_input(input)
    local queue = parse_to_queue(input)
    process_queue(queue)
end

-- Hook called by Go
function on_output(line)
    -- (Trigger logic goes here)
    return line
end
6. User Scripts Example
user.lua
code
Lua
-- Standard Aliases
add_alias("tt", "take torch")

-- Recursive Alias
add_alias("path_to_fountain", "s;s;w;open gate;n")

-- Bot logic (Alias with Wait)
-- When user types 'bot', it expands.
-- Expansion hits #wait, which calls Go's timer, which calls back Lua.
add_alias("bot", "kill orc; #wait 3; get coins; #wait 1; s")
Summary of "First Principles" Design
Center of Gravity: The Orchestrator loop. It is the single source of truth for the Lua state.
Thread Safety: Achieved by confinement. Only one goroutine ever touches L *lua.LState.
Concurrency: Channels allow the Network and UI to run at full speed without being blocked by Lua logic.
Timers: Handled by Go's native time.AfterFunc, injecting an event back into the Orchestrator. This enables #wait to work purely asynchronously.
Logic: Lua handles the command logic (Aliases, splitting, recursion) because that is dynamic business logic. Go handles the machinery (TCP, TTY, Time).
