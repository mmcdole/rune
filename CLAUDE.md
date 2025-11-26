# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Build the binary
go build ./cmd/rune/

# Run directly
go run ./cmd/rune/

# Run with user scripts
./rune user_script.lua another_script.lua
```

## Architecture Overview

Rune is a MUD (Multi-User Dungeon) client built with Go for system-level operations and Lua for business logic/scripting. It uses an **Actor Model architecture** where a single Orchestrator goroutine owns the Lua state and processes all events sequentially via channels.

### Core Design Principles

- **Single Lua state ownership**: One goroutine (the Orchestrator) owns and accesses the Lua state
- **Channel-based communication**: Thread safety through message passing, not mutexes
- **No blocking**: Each component runs independently with buffered channels
- **Kernel philosophy**: Go is the kernel (I/O, Memory, Concurrency), Lua is user space (Logic, Features, Presentation)

### Component Structure

```
cmd/rune/main.go       - Orchestrator: event loop that routes between UI, Network, and Lua
engine/lua.go          - LuaEngine: wraps gopher-lua, registers host functions under `rune` namespace
mud/types.go           - Core interfaces (ScriptEngine, Network, UI), event types, and action constants
network/tcp_client.go  - TCP client with telnet protocol support
network/telnet.go      - Telnet protocol buffer and negotiation
ui/console.go          - Console UI (stdin/stdout)
scripts/               - Lua scripts (core scripts embedded in binary via go:embed)
```

### Event Flow

```
User Input -> InputChan -> Orchestrator -> Lua on_input() -> NetSendChan -> Network
Server Line -> ServerChan -> Orchestrator -> Lua on_output() -> RenderChan -> UI
Server Prompt -> ServerChan -> Orchestrator -> Lua on_prompt() -> UI.RenderPrompt()
Timer -> TimerChan -> Orchestrator -> Lua ExecuteCallback()
Control -> EventChan -> Orchestrator -> CallHook() -> Lua on_sys_*()
```

### Event Types

- `EventUserInput` - User typed input
- `EventServerLine` - Complete line from server (ended with \n)
- `EventServerPrompt` - Partial line/prompt (no \n, possibly GA/EOR terminated)
- `EventTimer` - Timer callback
- `EventSystemControl` - Control operations (quit, connect, disconnect, reload, load)

### Action Constants

Use `mud.Action*` constants instead of string literals: `ActionQuit`, `ActionConnect`, `ActionDisconnect`, `ActionReload`, `ActionLoad`

### Lua API (rune namespace)

Go provides internal primitives (`rune._*`), wrapped by Lua for the public API:

**Core:**
- `rune.send(text)` - Process aliases and send to server
- `rune.send_raw(text)` - Bypass alias processing, write directly to socket
- `rune.print(text)` - Output text to local display
- `rune.quit()` - Exit the client
- `rune.connect(address)` - Connect to server
- `rune.disconnect()` - Disconnect from server
- `rune.reload()` - Reload all scripts
- `rune.load(path)` - Load a Lua script

**Timers:**
- `rune.timer.after(seconds, callback)` - Schedule delayed callback
- `rune.timer.every(seconds, callback)` - Schedule repeating callback, returns timer ID
- `rune.timer.cancel(id)` - Cancel a repeating timer
- `rune.timer.cancel_all()` - Cancel all repeating timers
- `rune.delay(seconds, action)` - Convenience: delay a command string or function

**Regex:**
- `rune.regex.match(pattern, text)` - Match using Go's regexp (cached)

**UI:**
- `rune.status.set(text)` - Set status bar
- `rune.pane.create(name)`, `rune.pane.write(name, text)`, `rune.pane.toggle(name)`, `rune.pane.clear(name)`, `rune.pane.bind(key, name)`
- `rune.infobar.set(text)` - Set info bar

**Config:**
- `rune.config_dir` - Path to ~/.config/rune

### Lua Hook Functions

Lua implements these for the Orchestrator to call:
- `on_input(text)` - Handle user input
- `on_output(text)` - Handle server output, return nil to gag
- `on_prompt(text)` - Handle server prompts (optional)

### System Hooks

Go calls these to notify Lua of system events (defined in `00_init.lua`, users can override):
- `on_sys_connecting(addr)` - Before connection attempt
- `on_sys_connected(addr)` - After successful connection
- `on_sys_disconnecting()` - Before disconnect
- `on_sys_disconnected()` - After disconnect
- `on_sys_reloading()` - Before script reload
- `on_sys_reloaded()` - After script reload
- `on_sys_loaded(path)` - After loading a script
- `on_sys_error(msg)` - On any system error

## Lua Scripting System

Core scripts in `scripts/core/` load in numeric order (00_, 05_, 10_, 20_, 30_, 40_) and provide:

- **Aliases**: `rune.alias.add(key, value)`, `rune.alias.remove(key)`, `rune.alias.list()`, `rune.alias.get(key)`, `rune.alias.run(name, args)`
- **Triggers**: `rune.trigger.add(pattern, action, {gag, enabled, regex})`, `rune.trigger.remove(id)`, `rune.trigger.list()`, `rune.trigger.enable(id, bool)`, `rune.trigger.process(line)`
- **Commands**: Semicolon-separated, use `rune.delay()` for async timing in function aliases
- **TinTin++ Syntax**: `#N command` expands to N repetitions (e.g., `#3 north` → `north;north;north`)
- **Slash Commands**: `/connect`, `/disconnect`, `/reconnect`, `/load`, `/reload`, `/lua`, `/aliases`, `/triggers`, `/test`, `/rmtrigger`, `/help`, `/quit`

User scripts auto-load from `~/.config/rune/init.lua` at startup.

## Current Development State

- No test suite exists
- UI is basic console - could upgrade to TUI

## Dependencies

- Go 1.25.4
- github.com/yuin/gopher-lua v1.1.1
