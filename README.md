# Rune

A modern MUD client built with Go and Lua.

Rune combines Go's performance and concurrency with Lua's flexibility for scripting. The architecture follows a kernel philosophy: Go handles I/O, memory, and concurrency while Lua handles logic, features, and presentation.

## Features

Scripting:

- **Aliases** - Command expansion with exact match or regex patterns
- **Triggers** - React to server output with pattern matching and gags
- **Timers** - One-shot and repeating timers with named management
- **Hooks** - Event system for connecting to input/output pipeline
- **Key Bindings** - Bind keys to Lua callbacks
- **Groups** - Master switches to enable/disable sets of aliases/triggers/timers
- **Command Chaining** - `kill rat;loot` sends both; `#3 {kill rat;loot}` repeats
- **Robust Scripting** - Watchdog interrupts runaway scripts; failing callbacks are quarantined individually instead of taking the client down

Protocols:

- **GMCP** - Structured server data (vitals, room info, channels) with a scriptable `rune.gmcp` API and automatic `Core.Hello` handshake
- **MCCP2** - Transparent zlib stream compression
- **TLS** - Encrypted connections (`tls://host:port`), including self-signed certs
- **Modern Telnet** - TTYPE/MTTS, NAWS, CHARSET (UTF-8), and MNES identification out of the box

Quality of life:

- **Worlds** - Named server bookmarks; `/connect` with no arguments opens a picker
- **Session Logging** - `/log` writes an ANSI-stripped transcript that reads like the screen
- **Durable Storage** - `rune.store` persists structured values across restarts
- **Tab Completion** - Word cache from server output with match cycling
- **History** - Zsh-style prefix-matching navigation
- **Panes & Bars** - Multiple output buffers with scrollback, reactive status bars

## Installation

```bash
go install github.com/mmcdole/rune/cmd/rune@latest
```

Or build from source:

```bash
git clone https://github.com/mmcdole/rune
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

-- GMCP: subscribe to vitals and react to them
rune.gmcp.subscribe("Char")
rune.gmcp.on("Char.Vitals", function(data)
    if data.hp and data.maxhp and data.hp < data.maxhp * 0.25 then
        rune.echo(rune.style.red("[LOW HP] " .. data.hp .. "/" .. data.maxhp))
    end
end)
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
| `/gmcp [send <package> [json]]` | GMCP status, or send a message for debugging |
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
