---
title: Custom Slash Commands
description: Add your own /commands. They join /help and the command picker automatically.
---

```lua
rune.command.add("greet", function(args)
    rune.send("say Hello, " .. (args ~= "" and args or "everyone") .. "!")
end, "Greet someone")
```

`/greet Bob` now works, appears in `/help`, and shows up in the `/` picker
with its description. All of that is generated from the registry, so the
listings never drift from what is actually registered.

## Creating

```lua
rune.command.add(name, handler, description?, opts?)
```

The handler is a function; it receives everything after `/name ` as a
single string (`""` when there are no arguments). The command name doubles
as its registry name.

## Options

Commands take the [common option](/scripting/model/#options) `group`.
The command name doubles as the registry `name`, so re-adding a name
replaces it.

## Examples

A command with subcommands:

```lua
rune.command.add("pather", function(args)
    local sub, rest = args:match("^(%S*)%s*(.*)$")
    if sub == "go" then
        pather.go(rest)
    elseif sub == "stop" then
        pather.stop()
    else
        rune.echo("[Usage] /pather go <place> | /pather stop")
    end
end, "Walk saved paths")
```

Overriding a built-in. Re-adding a name replaces it, so you can wrap:

```lua
local quit = rune.command.get("quit")
rune.command.add("quit", function(args)
    rune.send("save")
    quit(args)
end, "Save, then exit")
```

## Managing

By name: `rune.command.enable/disable/remove(name)`, plus
`rune.command.get(name)` for the raw handler — full signatures in the
[rune.command reference](/reference/api/command/). In the client, `/help`
lists every command, including script-added ones, with descriptions and
sources.

## Gotchas

- Commands are [quarantined](/scripting/model/#quarantine) individually:
  a broken handler can never take down input handling. A disabled command
  still consumes its input (with an error message).
- Unknown commands report `[Error] Unknown command: /x` and are never sent
  to the server. Use `/raw /text` if a game actually wants a literal slash.

**Related:** [rune.command reference](/reference/api/command/),
[Aliases](/scripting/aliases/),
[Built-in slash commands](/reference/slash-commands/)
