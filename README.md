# Rune

A modern MUD client built with Go and Lua.

Rune combines Go's performance and concurrency with Lua's flexibility for scripting. The architecture follows a kernel philosophy: Go handles I/O, memory, and concurrency while Lua handles logic, features, and presentation.

## Features

- **Aliases** - Command expansion with exact match or regex patterns
- **Triggers** - React to server output with pattern matching and gags
- **Timers** - One-shot and repeating timers with named management
- **Hooks** - Event system for connecting to input/output pipeline
- **Key Bindings** - Bind keys to Lua callbacks
- **Groups** - Master switches to enable/disable sets of aliases/triggers/timers
- **Worlds** - Named server bookmarks; `/connect` with no arguments opens a picker
- **TLS** - Encrypted connections (`tls://host:port`), including self-signed certs
- **Session Logging** - `/log` writes an ANSI-stripped transcript that reads like the screen
- **Durable Storage** - `rune.store` persists structured values across restarts
- **Tab Completion** - Word cache from server output with match cycling
- **History** - Zsh-style prefix-matching navigation
- **Panes & Bars** - Multiple output buffers with scrollback, reactive status bars
- **Robust Scripting** - Watchdog interrupts runaway scripts; failing callbacks are quarantined individually instead of taking the client down
- **TinTin++ Syntax** - `#3 north` expands to `north;north;north`

## Installation

```bash
go install github.com/drake/rune/cmd/rune@latest
```

Or build from source:

```bash
git clone https://github.com/drake/rune
cd rune
go build ./cmd/rune/
```

## Quick Start

```bash
# Run rune
./rune

# Connect to a MUD
/connect example.mud.com 4000

# Save it as a world, and reconnect by name from now on
/world add example example.mud.com 4000
/connect example

# Log the session
/log start

# Load a script
/load myscript.lua
```

## Configuration

User scripts are loaded from `~/.config/rune/init.lua` at startup.

Example `init.lua`:

```lua
-- Auto-connect
rune.connect("example.mud.com:4000")

-- Simple alias
rune.alias.exact("hp", "cast 'heal' self")

-- Regex alias with captures
rune.alias.regex("^kill (.+)$", "attack %1; murder %1")

-- Trigger to highlight damage
rune.trigger.contains("You are hit", function(matches, ctx)
    rune.echo(rune.style.red(ctx.line:clean()))
end, { gag = true })

-- Repeating timer
rune.timer.every(60, "save", { name = "autosave" })
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Up/Down` | History navigation (prefix-matching) |
| `Tab` | Cycle through completions |
| `Ctrl+R` | Search history |
| `Ctrl+T` | Search aliases |
| `/` | Slash command picker |
| `Ctrl+E` | Open input in $EDITOR |
| `PageUp/PageDown` | Scroll output |
| `Ctrl+C` (2x) | Quit |

## Slash Commands

| Command | Description |
|---------|-------------|
| `/connect [world \| host port [tls\|tls+insecure] \| address]` | Connect; no arguments opens the world picker |
| `/disconnect` | Close connection |
| `/reconnect` | Reconnect to last server (survives restarts) |
| `/world add\|remove\|list` | Manage world bookmarks |
| `/worlds` | List saved worlds |
| `/log start [file]\|stop\|status` | Log the session to a file |
| `/load <path>` | Load a Lua script |
| `/reload` | Reload all scripts |
| `/lua <code>` | Execute Lua inline |
| `/aliases`, `/triggers`, `/timers` | List registrations |
| `/hooks`, `/binds`, `/bars` | List registrations |
| `/groups` | List groups and their state |
| `/group <name> on\|off` | Toggle a group |
| `/raw <text>` | Send without alias expansion |
| `/echo <text>` | Print locally (never sent to the server) |
| `/test <line>` | Simulate server output against your triggers |
| `/version` | Show client version |
| `/help` | Show all commands |
| `/quit` | Exit |

## Documentation

- [Lua API Reference](docs/lua_doc.md)
- [Architecture Overview](docs/architecture.md)

## License

MIT License - see [LICENSE](LICENSE) for details.
