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

# Tests
go test ./...
```

## Architecture Overview

Rune is a MUD (Multi-User Dungeon) client built with Go for system-level operations and Lua for business logic/scripting. It uses an **Actor Model architecture** where a single Orchestrator goroutine (the Session) owns the Lua state and processes all events sequentially via channels.

### Core Design Principles

- **Single Lua state ownership**: One goroutine (the Session) owns and accesses the Lua state
- **Channel-based communication**: Thread safety through message passing, not mutexes
- **No blocking**: Each component runs independently with buffered channels
- **Kernel philosophy**: Go is the kernel (I/O, Memory, Concurrency), Lua is user space (Logic, Features, Presentation)

### Go/Lua Boundary Conventions

These rules keep the boundary consistent; follow them when adding APIs:

- **Go registers only `rune._*` primitives** (`_send_raw`, `_timer`, `_input`, `_ui`, ...). Every public name (`rune.send`, `rune.input.get`, `rune.ui.bar`, ...) is defined in Lua, even when the wrapper is thin. The Lua core in `lua/core/` IS the public API surface. The only non-underscore fields Go sets are `rune.config_dir` and `rune.version` (data, not API; version is single-sourced from the `version` package so TTYPE/MNES cannot drift from `/version`).
- **Registries live in Lua** on the shared factory (`rune.registry.new`, `15_registry.lua`). Hooks, timers, aliases, triggers, binds, bars, and slash commands all get handles, upsert-by-name, groups, priorities, source attribution, and failure quarantine from one implementation. Go dispatches through internal entry points (`rune.hooks.call`, `rune.binds._dispatch`, `rune.bars._render_all`, `rune.timer._fire`). Dispatch loops that keep iterating after a user callback runs iterate `Registry:snapshot()`, so callbacks may add/remove entries mid-dispatch safely.
- **Presentation belongs to Lua** via `rune.style` (`05_style.lua`). Even the local-echo styling (`"> "` prefix) is a Lua handler on the `"echo"` hook. Go colors only its last-resort degraded-path messages, through `text.Red`/`text.Green` - raw escape codes live in exactly one file per language.
- **Key policy**: Go handles keys only while a UI-internal mode is active (picker capture/cancel) plus Enter-to-submit. Everything else is a Lua bind. Bound printable keys fire only when the input is empty; Go's scroll-key handler is a fallback for unbound keys (keeps degraded mode scrollable).
- **Error convention**: Go primitives return `nil, err` for recoverable failures (send while disconnected, missing file, bad pattern); raising is reserved for programmer errors (wrong argument types).

### Script Robustness

- **Watchdog**: every Go→Lua entry runs under `Engine.guard` with a deadline (`Engine.CallTimeout`, default 5s). Runaway scripts are interrupted with an error; the VM stays usable. Nested entries share the outermost deadline. Host calls that legitimately block on the user (e.g. `open_editor` running `$EDITOR`) run under `Engine.pauseWatchdog`, which detaches the deadline and re-arms a fresh one.
- **Handler isolation**: `rune.hooks.call` runs each handler under `pcall`; a throwing handler is reported and skipped, not allowed to abort the chain.
- **Failure quarantine**: `rune.guarded_call(label, data, fn, ...)` tracks consecutive failures on a registry entry and disables it after 3 in a row. Used by hooks, triggers, aliases, timers, binds, bar renderers, and slash commands. Commands are quarantined individually (`rune.command.dispatch`), so a broken user command can never disable the core input hook.
- **Degraded mode**: if `rune.hooks.call` is unavailable (core script failed, or a user script clobbered `rune.hooks`), the client degrades to a plain telnet client instead of crashing - output passes through raw, input goes to the server, and `/quit` + `/reload` still work.
- **Source attribution**: registrations record the registering script's `file:line` (`rune.caller_source`); it appears in error messages and the `/hooks`, `/triggers`, `/aliases`, `/timers`, `/binds`, `/bars` listings.
- **Engine-level failures** route through `Engine.reportError` → the Lua `"error"` event, with a re-entrancy guard that falls back to direct printing.

### Component Structure

```
cmd/rune/main.go              - Bootstrap: creates Session and runs UI
config/config.go              - Config dir resolution (XDG/APPDATA)
event/event.go                - Session event types and payloads
session/                      - Session: event loop, implements lua.Host
  session.go                  - Orchestrator, Network interface, boot
  lua_*.go                    - Host implementation (network, ui, timers,
                                system, history, session, store, log, state)
lua/                          - Lua runtime package
  engine.go                   - Engine: wraps gopher-lua, watchdog, dispatch
  api_*.go                    - rune._* primitive registration
  host.go                     - Host interface: bridge between Engine and Session
  core/*.lua                  - Embedded Lua core (the public API)
text/                         - Line type, ANSI stripper, degraded-path colors
timer/service.go              - Timer service (scheduling only; callbacks in Lua)
network/                      - TCP client, telnet parser, output buffering,
                                identity responders (negotiate.go), MCCP2, GMCP
version/version.go            - Client name/version (TTYPE, MNES, rune.version)
ui/                           - UI interface, messages, TUI implementation
```

### Event Flow

```
User Input -> UI input chan -> Session -> rune.hooks.call("input")  -> network
Server Line -> net output   -> Session -> rune.hooks.call("output") -> UI print
Server Prompt -> net output -> Session -> rune.hooks.call("prompt") -> UI prompt overlay
Timer fire -> timer events  -> Session -> rune.timer._fire(id)
Key bind -> UI outbound     -> Session -> rune.binds._dispatch(key)
Bar tick (250ms)            -> Session -> rune.bars._render_all(width) -> UI bars
```

### Event Types (event/event.go)

- `UserInput` - User typed input
- `NetLine` - Complete line from server (ended with \n)
- `NetPrompt` - Partial line/prompt (no \n, possibly GA/EOR terminated)
- `SysDisconnect` - Connection closed
- `AsyncResult` - Deferred callback execution (used by reload and connect)

## Lua Core Scripts (lua/core/, loaded in numeric order)

- `00_init.lua` - Config, guarded_call, caller_source, line objects, capture substitution, primitive wrappers (send_raw, session/store, history, rune.ui, rune.state proxy)
- `05_style.lua` - `rune.style` ANSI helpers (the one place Lua writes escape codes)
- `10_regex.lua` - Cached Go-regexp matching (bounded cache), `validate`
- `15_registry.lua` - Shared registry factory (`rune.registry.new`)
- `20_hooks.lua` - Hook registry + `rune.hooks.call` dispatcher
- `25_groups.lua` - Group master switches
- `30_binds.lua` - Key bindings (`rune.bind`, `rune.binds`)
- `35_bars.lua` - Bar renderers (`rune.ui.bar`, `rune.bars`)
- `40_timers.lua` - Timers (owns id→callback map; Go only schedules)
- `45_aliases.lua` - Aliases (exact + regex)
- `50_triggers.lua` - Triggers (exact/starts/contains/regex, gag, raw)
- `55_commands.lua` - Slash commands (registry-based; `rune.command.dispatch` quarantines each command individually; `/help` is generated from the registry)
- `57_log.lua` - Session logging (`rune.log`, the `log-output`/`log-echo` policy hooks, `/log`); the file handle is Go-owned so logging survives `/reload`
- `58_worlds.lua` - World bookmarks (`rune.world`, `/world`, `/worlds`), stored durably via `rune.store`
- `59_gmcp.lua` - GMCP policy (`rune.gmcp` handlers/subscriptions on the shared registry, the Core.Hello handshake on `"gmcp_enabled"`, `/gmcp`); Go owns the option-201 transport and JSON bridge
- `60_send.lua` - Command expansion (`;` splitting, `#N` repeats anchored at command position), core input/output/prompt handlers
- `65_events.lua` - Default system event handlers (including the `"echo"` styler)
- `70_input.lua` - Input wrappers, history navigation, word ops, tab completion
- `75_ui.lua` - Panes, status bar, default binds and pickers

## Lua API (rune namespace) - highlights

Full reference: `website/src/content/docs/reference/api/` (one page per
namespace; published at runemud.com/reference/api/). A new public `rune.*`
function must be added there or `lua/api_docs_coverage_test.go` fails.
Go primitives (`rune._*`) are internal.

- **Core**: `rune.send`, `rune.send_raw`, `rune.echo`, `rune.connect`, `rune.disconnect`, `rune.load`, `rune.reload`, `rune.quit`, `rune.config_dir`, `rune.version`. Connect addresses take an optional scheme: `host:port` (plain, default), `tls://host:port`, `tls+insecure://host:port` (self-signed certs)
- **Registries** (all return handles with `:enable/:disable/:remove/:name/:group`; opts `{name, group, priority, once}`):
  - `rune.alias.exact/regex(pattern, action, opts?)`
  - `rune.trigger.exact/starts/contains/regex(pattern, action, opts?)` (+ `gag`, `raw` opts)
  - `rune.timer.after/every(seconds, action, opts?)` - `every` is fixed-interval
  - `rune.hooks.on(event, handler, opts?)`
  - `rune.bind(key, callback, opts?)` / `rune.unbind(key)`
  - `rune.ui.bar(name, render_fn, opts?)`
  - `rune.command.add(name, handler, description?, opts?)`
- **Groups**: `rune.group.enable/disable/is_enabled/list` - an item fires only if itself enabled AND its group enabled (all registries honor this)
- **Regex**: `rune.regex.match(pattern, text)` (cached), `rune.regex.validate(pattern)`, `rune.regex.compile(pattern)`. `trigger.regex`/`alias.regex` validate eagerly and raise on bad patterns.
- **UI**: `rune.ui.layout{top=..., bottom=...}`, `rune.ui.refresh_bars()`, `rune.ui.picker.show(opts)`, `rune.pane.*`
- **Input**: `rune.input.get/set/get_cursor/set_cursor/open_editor/word_left/word_right/delete_word`
- **State**: `rune.state` (read-only proxy: connected, address, scroll_mode, scroll_lines, width, height)
- **Storage** (two Go-owned tiers; the name encodes the lifetime): `rune.session.set/get/delete` - string store that survives `/reload` but not exit; `rune.store.set/get/delete` - durable store backed by `<config>/store.json` (atomic write-through), values may be strings/numbers/booleans/JSON-able tables, `set(key, nil)` deletes, unstorable values return `nil, err`
- **Worlds**: `rune.world.add/remove/get/list` - named server bookmarks in `rune.store` under `"worlds"`; `/connect <name>` resolves them first, bare `/connect` opens a picker over them
- **Logging**: `rune.log.start/stop/status/write` - session log to file. The handle is Go-owned (survives `/reload`, closed on exit); what gets written is Lua policy in `57_log.lua` (post-trigger output + input echo, ANSI-stripped; gagged lines and prompts excluded)
- **GMCP**: `rune.gmcp.on(package, handler, opts?)` (registry-based: quarantine, groups, source attribution), `rune.gmcp.send(package, value?)` (JSON-able Lua values), `send_raw`, `subscribe/unsubscribe` (maintains `Core.Supports.Set`), `is_enabled`. Handlers get `(decoded_data, package)`; package matching is case-insensitive. Malformed server JSON is reported and dropped in Go
- **Style**: `rune.style.red/green/yellow/.../bold/dim/inverse`
- **Lines**: output/prompt handlers receive line objects (`:raw()`, `:clean()`); `rune.line.new(text)` builds one
- **History**: `rune.history.get/add`

### Hook Events

Data-flow: `"output"`, `"prompt"`, `"echo"` support returning `false` to gag or a string to rewrite (rewrites CHAIN to subsequent handlers; the core `"echo"` handler adds the `"> "` styling). `"input"` supports only `false` (consume) - string returns are ignored, and the core input handler at priority 100 always consumes, so custom input handlers must register below 100.
Notifications: `"ready"`, `"connecting"`, `"connected"`, `"disconnecting"`, `"disconnected"`, `"reloading"`, `"reloaded"`, `"loaded"`, `"error"`, `"input_changed"`, `"gmcp"` (catch-all: `package, data, raw`), `"gmcp_enabled"` (core handler sends Core.Hello).

### Slash Commands

`/connect` (a world name, `<host> <port> [tls|tls+insecure]`, or a single address; no args opens the world picker), `/disconnect`, `/reconnect` (survives restarts via `rune.store`), `/world add|remove|list`, `/worlds`, `/load`, `/reload`, `/lua`, `/log` (`start [file]` / `stop` / `status`), `/aliases`, `/triggers`, `/timers`, `/hooks`, `/binds`, `/bars`, `/groups`, `/group <name> on|off`, `/gmcp` (status; `send <pkg> [json]` to debug), `/raw`, `/echo`, `/test`, `/version`, `/help`, `/quit`

User scripts auto-load from `~/.config/rune/init.lua` at startup.

## Testing

- `lua/` - engine tests (watchdog, quarantine, degraded mode, dispatch round trips) + JSON-driven feature tests in `lua/testdata/*_tests.json`
- `session/` - event-loop tests against mock Network/UI (prompt lifecycle, reload, echo negotiation)
- `network/` - telnet parser and option negotiation tests
- `timer/`, `text/` - unit tests

## Telnet Notes

The default compatibility table advertises ONLY implemented options: Echo, SGA, EOR, TTYPE/MTTS, NAWS, CHARSET, NEW-ENVIRON/MNES (identity responders in `network/negotiate.go` - pure functions, byte-exact tests), MCCP2 (zlib read path in client.go; the source is a byte-exact `bufio.Reader`, so a clean stream end resumes plain telnet), and GMCP (option 201; framing in Go, policy in `59_gmcp.lua`). Never `Support()` an option without implementing its behavior - agreeing to an option without honoring its subnegotiations breaks real servers (MCCP3, MSSP, ZMP, Linemode stay refused). All socket writes go through the connection's single writeLoop. The parser accepts subnegotiations for options enabled on either side (server-offered GMCP/MCCP are remote; client-answered TTYPE/NAWS are local).

## Releasing

Versions come from git tags: `git tag vX.Y.Z && git push --tags` runs the release workflow (goreleaser), which builds linux/darwin/windows binaries and stamps `version.Number` via ldflags. The in-repo default (`X.Y.Z-dev`) marks untagged builds. Tag only after the manual QA checklist passes.

## Dependencies

- Go 1.25.4
- github.com/yuin/gopher-lua v1.1.1
- github.com/charmbracelet/bubbletea (TUI)
