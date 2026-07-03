---
title: Slash Commands
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

| Option | Effect |
|---|---|
| `group` | Adds the command to a group. Toggle the set with `/group <name> on\|off`. |

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

```lua
rune.command.remove(name)
rune.command.enable(name)   rune.command.disable(name)
rune.command.get(name)      -- the raw handler function
rune.command.list()
```

In the client, `/help` lists every command, including script-added ones,
with descriptions and sources.

## Gotchas

- Commands are quarantined individually: a handler that errors three times
  in a row is disabled with a notice and can never take down input
  handling. A disabled command still consumes its input (with an error
  message). Fix the code, then re-enable it with
  `rune.command.enable(name)`.
- Unknown commands report `[Error] Unknown command: /x` and are never sent
  to the server. Use `/raw /text` if a game actually wants a literal slash.

**Related:** [Aliases](/scripting/aliases/),
[Slash command reference](/reference/slash-commands/)
