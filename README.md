# Rune

A modern MUD client built with Go and Lua.

[![CI](https://github.com/mmcdole/rune/actions/workflows/ci.yml/badge.svg)](https://github.com/mmcdole/rune/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/mmcdole/rune/graph/badge.svg)](https://codecov.io/gh/mmcdole/rune)

![One rune session: the world picker, tab completion cycling, history search, combat, and the multiline verbatim composer](.github/montage.gif)

Rune combines Go's performance and concurrency with Lua's flexibility for scripting. The architecture follows a kernel philosophy: Go handles I/O, memory, and concurrency while Lua handles logic, features, and presentation.

Guides, cookbook recipes, and the full API reference live at **[runemud.com](https://runemud.com)**.

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

Download a binary for Linux, macOS, or Windows from the
[releases page](https://github.com/mmcdole/rune/releases), unpack it,
and put `rune` on your PATH.

With Go installed, you can instead:

```bash
go install github.com/mmcdole/rune/cmd/rune@latest
```

Or build from source:

```bash
git clone https://github.com/mmcdole/rune
cd rune
go build ./cmd/rune/
```

See the [installation guide](https://runemud.com/getting-started/installation/)
for details.

## Quick Start

Connect straight from your shell:

```bash
rune mud.example.com 4000           # plain telnet
rune tls://mud.example.com:4000     # TLS
```

Then, inside the client:

```
/world add example mud.example.com 4000    save a bookmark
/connect example                           connect by name (or later: rune example)
/log start                                 log the session to a file
/help                                      everything else
```

User scripts load from `~/.config/rune/init.lua` at startup:

```lua
-- Alias with regex captures
rune.alias.regex("^kill (.+)$", "attack %1; murder %1")

-- Trigger: highlight a line in red
rune.trigger.contains("You are hit", function(matches, ctx)
    return rune.style.red(ctx.line:clean())
end)

-- GMCP: warn on low health
rune.gmcp.subscribe("Char")
rune.gmcp.on("Char.Vitals", function(data)
    if data.hp and data.maxhp and data.hp < data.maxhp * 0.25 then
        rune.echo(rune.style.red("[LOW HP] " .. data.hp .. "/" .. data.maxhp))
    end
end)
```

The [scripting basics](https://runemud.com/getting-started/scripting-basics/)
guide picks up from here.

## Documentation

Full guides, cookbook recipes, and reference live at
**[runemud.com](https://runemud.com)**:

- [First session](https://runemud.com/getting-started/first-session/) and [scripting basics](https://runemud.com/getting-started/scripting-basics/)
- [Lua API reference](https://runemud.com/reference/api/) — every `rune.*` namespace
- [Slash commands](https://runemud.com/reference/slash-commands/) and [keyboard shortcuts](https://runemud.com/interface/input/)
- [Migrating from another MUD client](https://runemud.com/getting-started/migrating/)

In-repo reference: the [architecture overview](docs/architecture.md).
The API reference source lives at `website/src/content/docs/reference/api/`.

## License

MIT License - see [LICENSE](LICENSE) for details.
