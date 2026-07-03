---
title: Triggers
description: React to server output with four match modes and string or function actions that can rewrite, gag, and capture.
---

A trigger fires when a line arrives from the server. Two decisions define
one: how it matches (exact line, prefix, substring, or regex) and what it
does (a string sent as a command, or a Lua function).

A string action is a canned response:

```lua
rune.trigger.contains("You are hungry", "eat bread")
```

A function can inspect the line and rewrite or hide it:

```lua
rune.trigger.contains("You are hit", function(matches, ctx)
    return rune.style.red(ctx.line:clean())  -- rewrite: highlight in red
end)
```

Use a string when a match should send commands and nothing else. Use a
function when you need logic, state, or control over the line itself; only
a function can rewrite or gag.

## Creating

```lua
rune.trigger.exact(line, action, opts?)      -- whole line matches exactly
rune.trigger.starts(prefix, action, opts?)   -- line starts with prefix
rune.trigger.contains(text, action, opts?)   -- line contains text
rune.trigger.regex(pattern, action, opts?)   -- Go regexp, with captures
```

Matching runs against the clean line (ANSI stripped), so patterns don't
fight color codes. Pass `raw = true` to match the raw line instead. Regex
patterns are validated at registration: a bad pattern raises immediately
instead of failing silently at match time.

## Actions

**A string** is sent as a command (`;` chaining and aliases apply). For
`regex` triggers, `%1`, `%2`, and so on are substituted from captures:

```lua
rune.trigger.regex("^(\\w+) gives you a (.+)\\.$", "thank %1")
```

**A function** is called with `(matches, ctx)`. `matches` is the array of
regex captures (empty for the literal modes). `ctx` carries `line`, `name`,
`group`, `type`, and `matches`; `ctx.line` is a line object with `:raw()`
and `:clean()`. The return value controls the output:

| Return | Effect |
|---|---|
| `nil` | Line passes through unchanged |
| a string | Rewrites the line; this is how you highlight |
| `false` | Gags the line |

A string action never touches the line; it only sends. Rewriting and
gagging are function-return features, plus the `gag` option.

## Options

| Option | Effect |
|---|---|
| `gag` | Hides matching lines (no action required). |
| `raw` | Matches against the raw line, ANSI codes included. |
| `name` | Unique name. Registering the same name again replaces the old trigger. |
| `group` | Adds the trigger to a group. Toggle the set with `/group <name> on\|off`. |
| `priority` | Order among triggers. Lower runs first (default 50). |
| `once` | Fires a single time, then removes itself. |

## Examples

Pure gag, no action needed:

```lua
rune.trigger.contains("The shopkeeper hums", nil, { gag = true })
```

Capture and act:

```lua
rune.trigger.regex("^(\\w+) tells you: follow me$", function(m)
    rune.send("follow " .. m[1])
end)
```

Mirror to a pane (gag from the main window, keep it in the pane):

```lua
rune.trigger.regex("^\\[Auction\\] (.+)$", function(m, ctx)
    rune.pane.write("auctions", ctx.line:raw())
    return false
end)
```

One-shot login prompt:

```lua
rune.trigger.contains("What is your name", function()
    rune.send("Ragnar")
end, { once = true })
```

Rewrites chain: later triggers match against (and receive) the rewritten
line, so a highlighter and a tagger compose:

```lua
rune.trigger.contains("dragon", function(m, ctx)
    return rune.style.red(ctx.line:clean())
end)
rune.trigger.contains("dragon", function(m, ctx)
    return "!! " .. ctx.line:raw()
end)
-- output: "!! <red line>"
```

Test any of this without a server: `/test <line>` runs a line through your
triggers and shows what would happen.

## Managing

Every constructor returns a handle:

```lua
local h = rune.trigger.contains("hungry", "eat bread", { name = "auto-eat" })
h:disable()  h:enable()  h:remove()
```

By name: `rune.trigger.disable/enable/remove(name)`, and
`rune.trigger.list()` returns everything registered. In the client,
`/triggers` shows every trigger with its state, mode, flags, group, and the
`file:line` that registered it.

## Gotchas

- Patterns are Go regexp (RE2), not Lua patterns: `\\d`, `\\w`, and `\\s`
  work, backreferences and lookaround do not.
- Prompts (partial lines) run through triggers too. Anchor with `^...$`
  when you only want complete lines.
- A trigger that errors three times in a row is quarantined: it is disabled
  with a notice instead of firing an error on every line. Fix the code,
  then re-enable it with `rune.trigger.enable(name)`.

**Related:** [Aliases](/scripting/aliases/),
[Hooks & Events](/scripting/hooks/),
[Panes](/interface/panes/)
