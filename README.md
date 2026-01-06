# Rune

A modern MUD client built with Go and Lua.

Rune combines Go's performance and concurrency with Lua's flexibility for scripting. The architecture follows a kernel philosophy: Go handles I/O, memory, and concurrency while Lua handles logic, features, and presentation.

## Features

- **Aliases** - Command expansion with exact match or regex patterns
- **Triggers** - React to server output with pattern matching and gags
- **Timers** - One-shot and repeating timers with named management
- **Hooks** - Event system for connecting to input/output pipeline
- **Groups** - Master switches to enable/disable sets of aliases/triggers/timers
- **Tab Completion** - Word cache from server output with ghost text
- **History** - Zsh-style prefix-matching navigation
- **Panes** - Multiple output buffers with scrollback
- **Reactive Status Bar** - Customizable status display
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
    rune.echo("\027[31m" .. ctx.line:raw() .. "\027[0m")
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
| `/connect <host> <port>` | Connect to server |
| `/disconnect` | Close connection |
| `/reconnect` | Reconnect to last server |
| `/load <path>` | Load a Lua script |
| `/reload` | Reload all scripts |
| `/lua <code>` | Execute Lua inline |
| `/aliases` | List all aliases |
| `/triggers` | List all triggers |
| `/timers` | List all timers |
| `/help` | Show help |
| `/quit` | Exit |

## Documentation

- [Lua API Reference](docs/lua_doc.md)
- [Architecture Overview](docs/architecture.md)
- [Layout System](docs/layout-system.md)

## License

MIT License - see [LICENSE](LICENSE) for details.
