---
title: rune.command
description: Full signatures for custom slash commands — registration, /help integration, per-command quarantine.
---

Slash commands add `/name` verbs to the input line. For a
task-oriented introduction, see [Slash Commands](/scripting/commands/).

## Quick reference

```lua
rune.command.add(name, handler, description?, opts?)  -- register /name
rune.command.remove(name)                             -- unregister; true if it existed
rune.command.get(name)                                -- the raw handler, or nil
rune.command.enable(name)                             -- re-enable (also recovers from quarantine)
rune.command.disable(name)                            -- disable without unregistering
rune.command.list()                                   -- array of {name, description, enabled, group, source}
```

`add` returns a [handle](/reference/api/#handles); `opts` accepts
`group` from the [common options](/reference/api/#options) (the item's
name is the command name itself).

### rune.command.add

```lua
rune.command.add(name, handler, description?, opts?) -> handle
```

- `name` (string) — the command name, without the slash. Re-adding the
  same name replaces the old handler (upsert).
- `handler` (function) — `function(args)`; `args` is everything after
  `/name ` as a single string (`""` when there are no arguments).
- `description` (string, optional) — shown in `/help` and the `/`
  command picker.
- `opts` (table, optional) — `{group = "..."}`.

```lua
rune.command.add("greet", function(args)
    rune.send("say Hello, " .. (args ~= "" and args or "everyone") .. "!")
end, "Greet someone")
```

`/help` and the `/` picker are generated from the registry, so
user-added commands appear in both automatically.

Each command is [quarantined](/reference/api/#quarantine) individually —
a command that throws three times in a row is disabled with a notice,
and input handling keeps working. Fix the error and
`rune.command.enable(name)` (or `/reload`) to recover.

## Managing

Standard registry management applies:
`rune.command.enable/disable/remove(name)`, `.list()` — see
[Registries](/reference/api/#managing). `/help` lists everything.

**Related:** [Slash Commands guide](/scripting/commands/) ·
[rune.alias](/reference/api/alias/) ·
[rune.bind](/reference/api/bind/) ·
[Slash Commands reference](/reference/slash-commands/)
