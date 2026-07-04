---
title: Triggers
description: React to server output with four match modes and string or function actions that run your own logic, send commands, and can rewrite or gag the line.
---

A trigger fires when a line arrives from the server. Two decisions define
one: how it matches (exact line, prefix, substring, or regex) and what it
does (a string sent as a command, or a Lua function).

A string action is a canned response:

```lua
rune.trigger.contains("You are hungry", "eat bread")
```

A function runs your own logic when the line matches: make decisions, track
state, send commands, call any API.

```lua
rune.trigger.regex("^Your health is (\\d+)%\\.$", function(m)
    if tonumber(m[1]) < 30 then
        rune.send("quaff heal")
    end
end)
```

A function can also shape the line itself: return a string to rewrite it
(this is how you highlight) or `false` to gag it.

Use a string when a match should send a fixed command and nothing else.
Use a function whenever you need logic, state, or control over the line;
only a function can do any of those.

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

Triggers take the [common options](/scripting/model/#options) — `name`,
`group`, `priority`, `once` — plus two of their own:

| Option | Effect |
|---|---|
| `gag` | Hides matching lines (no action required). |
| `raw` | Matches against the raw line, ANSI codes included. |
| `span` | Collects a multi-line message before firing. See [Multi-line triggers](#multi-line-triggers). |

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
triggers and shows what would happen. Multi-line spans collect across
`/test` invocations, one line per call — handy for exercising them
offline.

## Multi-line triggers

Plenty of server output spans lines: chat and tells that wrap, score
sheets, who lists, quest logs, mail. A plain trigger only ever sees
one line at a time; a `span` collects the block and fires the action
once, with everything.

For wrapped messages, the end of the block is usually a pattern on
the final line — many LPMuds leave the channel's ANSI color reset on
the last wrapped line (log raw output to find your server's marker):

```lua
rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
    rune.pane.write("chat", "[Tell] " .. m[1] .. ": " .. ctx.text)
end, { span = { to = "\\x1b\\[0?m\\s*$", raw = true, max = 8 } })
```

`span.to` is the regex for the line that ends the message, inclusive.
`span.raw` matches it against the raw line, since stripping removes
escape codes. `span.max` (default 8) is the safety cap when the
terminator never shows.

For fixed-shape blocks, skip `to` and use `max` alone — the span
collects exactly that many lines:

```lua
-- A score block that is always 4 lines
rune.trigger.starts("You have scored", function(m, ctx)
    parse_score(ctx.lines)
end, { span = { max = 4 } })
```

In the action, `ctx.text` is the joined text and `ctx.lines` has the
individual lines. The collected lines have already been displayed by
the time the action runs, so return values do nothing; `gag = true`
is the exception and hides every collected line as it arrives:

```lua
-- Silence a multi-line broadcast entirely
rune.trigger.starts("The town crier bellows:", nil,
    { gag = true, span = { to = "\\x1b\\[0?m\\s*$", raw = true } })
```

Full semantics: [rune.trigger reference](/reference/api/trigger/#multi-line-triggers).

## Managing

Every constructor returns a handle:

```lua
local h = rune.trigger.contains("hungry", "eat bread", { name = "auto-eat" })
h:disable()  h:enable()  h:remove()
```

By name: `rune.trigger.disable/enable/remove(name)` — the full management
suite is in the [API reference](/reference/api/#managing). In the client,
`/triggers` shows every trigger with its state, mode, flags, group, and the
`file:line` that registered it.

## Gotchas

- Patterns are Go regexp (RE2), not Lua patterns: `\\d`, `\\w`, and `\\s`
  work, backreferences and lookaround do not — see
  [rune.regex](/reference/api/regex/) for the syntax notes.
- Prompts (partial lines) run through triggers too. Anchor with `^...$`
  when you only want complete lines.
- A prompt ends any open multi-line span (the action fires with what
  was collected), and `/reload` discards open spans.
- A trigger that errors three times in a row is
  [quarantined](/scripting/model/#quarantine).

**Related:** [rune.trigger reference](/reference/api/trigger/),
[Aliases](/scripting/aliases/),
[Hooks & Events](/scripting/hooks/),
[Panes](/interface/panes/)
