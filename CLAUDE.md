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

### Script Robustness

- **Watchdog**: every Go→Lua entry runs under `Engine.guard` with a deadline (`Engine.CallTimeout`, default 5s). Runaway scripts (infinite loops) are interrupted with an error; the VM stays usable. Nested entries share the outermost deadline.
- **Error convention**: Go primitives return `nil, err` for recoverable failures (send while disconnected, missing file, bad pattern); raising is reserved for programmer errors (wrong argument types). Engine-level failures route through `Engine.reportError` → the Lua `"error"` event, with a re-entrancy guard that falls back to direct printing.
- **Handler isolation**: `rune.hooks.call` runs each handler under `pcall`; a throwing handler is reported and skipped, not allowed to abort the chain.
- **Failure quarantine**: `rune.guarded_call(label, data, fn, ...)` tracks consecutive failures on a registry entry and disables it after 3 in a row. Used by hooks, trigger actions, and timer actions; bar renderers get the same treatment in Go (`maxBarFailures`).
- **Degraded mode**: if `rune.hooks.call` is unavailable (core script failed, or a user script clobbered `rune.hooks`), the client degrades to a plain telnet client instead of crashing - output passes through raw, input goes to the server, and `/quit` + `/reload` still work.
- **Source attribution**: hooks, triggers, aliases, and timers record the registering script's `file:line` (`rune.caller_source`); it appears in error messages and `/hooks`, `/triggers`, `/aliases`, `/timers` listings.

### Component Structure

```
cmd/rune/main.go              - Bootstrap: creates Session and runs UI
session/session.go            - Session: orchestrates event loop, implements lua.Host
lua/                          - Lua runtime package
  engine.go                   - Engine: wraps gopher-lua, manages VM lifecycle
  api_*.go                    - Go→Lua bindings (core, timer, regex, ui)
  host.go                     - Host interface: bridge between Engine and Session
  core/*.lua                  - Embedded Lua scripts (aliases, triggers, hooks, etc.)
mud/types.go                  - Core interfaces (Network, UI), event types, action constants
network/tcp_client.go         - TCP client with telnet protocol support
network/telnet.go             - Telnet protocol buffer and negotiation
ui/                           - UI implementations (console and TUI)
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

- `UserInput` - User typed input
- `NetLine` - Complete line from server (ended with \n)
- `NetPrompt` - Partial line/prompt (no \n, possibly GA/EOR terminated)
- `SysQuit` - Quit the client
- `SysConnect` - Connect to server (Payload = address)
- `SysDisconnect` - Disconnect from server
- `SysReload` - Reload all scripts
- `SysLoadScript` - Load a script (Payload = path)
- `Timer` - Timer callback
- `AsyncResult` - Async work completion

### Lua API (rune namespace)

Go provides internal primitives (`rune._*`), wrapped by Lua for the public API:

**Core:**
- `rune.send(text)` - Process aliases and send to server
- `rune.send_raw(text)` - Bypass alias processing, write directly to socket; returns `true` or `nil, err` (failures are echoed)
- `rune.echo(text)` - Output text to local display
- `rune.quit()` - Exit the client
- `rune.connect(address)` - Connect to server
- `rune.disconnect()` - Disconnect from server
- `rune.reload()` - Reload all scripts
- `rune.load(path)` - Load a Lua script; returns `true` or `nil, err`

**Timers:**
- `rune.timer.after(seconds, callback)` - Schedule delayed callback
- `rune.timer.every(seconds, callback)` - Schedule repeating callback, returns timer ID
- `rune.timer.cancel(id)` - Cancel a repeating timer
- `rune.timer.cancel_all()` - Cancel all repeating timers
- `rune.delay(seconds, action)` - Convenience: delay a command string or function

**Regex:**
- `rune.regex.match(pattern, text)` - Match using Go's regexp (cached; invalid patterns are reported once and cached as failures)
- `rune.regex.validate(pattern)` - Check a pattern; returns `true` or `nil, err`. `rune.trigger.regex` and `rune.alias.regex` validate eagerly and raise on bad patterns.

**UI:**
- `rune.pane.create(name)`, `rune.pane.write(name, text)`, `rune.pane.toggle(name)`, `rune.pane.clear(name)`
- `rune.ui.bar(name, render_fn)` - Register a reactive bar renderer
- `rune.ui.layout({top=..., bottom=...})` - Set layout configuration

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

Core scripts in `lua/core/` load in numeric order (00_, 10_, 20_, 30_, 40_, 50_, 60_) and provide:

- **Aliases**: `rune.alias.add(key, value)`, `rune.alias.remove(key)`, `rune.alias.list()`, `rune.alias.get(key)`, `rune.alias.run(name, args)`
- **Triggers**: `rune.trigger.add(pattern, action, {gag, enabled, regex})`, `rune.trigger.remove(id)`, `rune.trigger.list()`, `rune.trigger.enable(id, bool)`, `rune.trigger.process(line)`
- **Commands**: Semicolon-separated, use `rune.delay()` for async timing in function aliases
- **TinTin++ Syntax**: `#N command` expands to N repetitions (e.g., `#3 north` → `north;north;north`)
- **Slash Commands**: `/connect`, `/disconnect`, `/reconnect`, `/load`, `/reload`, `/lua`, `/aliases`, `/triggers`, `/test`, `/rmtrigger`, `/help`, `/quit`

User scripts auto-load from `~/.config/rune/init.lua` at startup.

## Current Development State

- Test suite exists in `lua/testdata/` (JSON-driven tests)
- UI uses TUI (Bubble Tea) by default

## Dependencies

- Go 1.25.4
- github.com/yuin/gopher-lua v1.1.1
