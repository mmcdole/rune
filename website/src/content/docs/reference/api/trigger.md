---
title: rune.trigger
description: Full signatures for reacting to server output — match modes, actions, gag and rewrite.
---

Triggers match lines of server output and run actions. For a
task-oriented introduction, see [Triggers](/scripting/triggers/).

## Quick reference

```lua
rune.trigger.exact(line, action, opts?)      -- whole line matches exactly
rune.trigger.starts(prefix, action, opts?)   -- line starts with prefix
rune.trigger.contains(text, action, opts?)   -- line contains text
rune.trigger.regex(pattern, action, opts?)   -- Go regexp, with captures
```

All constructors return a [handle](/reference/api/#handles) and accept
the [common options](/reference/api/#options) plus `gag`, `raw`, `on`,
`confirmed_only`, and [`span`](#multi-line-triggers).

Triggers match completed output lines by default. Set `on = "prompt"` to
match live prompt-area updates instead.

## Matching

| Mode | Matches when | Captures |
|---|---|---|
| `exact` | the whole clean line equals `line` | none |
| `starts` | the line begins with `prefix` | none |
| `contains` | the line contains `text` | none |
| `regex` | the Go regexp matches | capture groups |

Matching runs against the clean (ANSI-stripped) line unless `raw = true`.
Triggers run in `priority` order (lower first); a rewrite from one
trigger is what later triggers match against.

### rune.trigger.regex

```lua
rune.trigger.regex(pattern, action, opts?) -> handle
```

- `pattern` (string) — Go regexp ([RE2](/reference/api/regex/), not Lua
  patterns). Validated at registration; a bad pattern raises immediately.
- `action` (string | function | nil) — a command string (`%1`…`%n`
  substituted from captures), or `function(matches, ctx)`. `nil` is
  allowed with `gag = true`.
- `opts` (table, optional) — [common options](/reference/api/#options)
  plus the options below.

```lua
rune.trigger.regex("^(\\w+) tells you: follow me$", function(m)
    rune.send("follow " .. m[1])
end)
```

## Actions and return values

A string action is sent as a command. A function action receives
`(matches, ctx)` — `ctx.line` is the [line object](/reference/api/state-lines/)
with `:raw()` and `:clean()`. Prompt-trigger actions also receive
`ctx.prompt_confirmed`, a boolean. The return value controls the line:

| Return | Effect |
|---|---|
| `nil` | Line passes through unchanged |
| string | Line is rewritten; later triggers see the new text |
| `false` | Line is gagged (hidden) |

This table does not apply to [multi-line triggers](#multi-line-triggers):
their actions fire after the collected lines have been displayed, so
return values are ignored.

## Options

Beyond the [common options](/reference/api/#options):

| Option | Type | Default | Description |
|---|---|---|---|
| `gag` | bool | false | Hide the matching line (equivalent to returning `false`) |
| `raw` | bool | false | Match against the raw line, ANSI codes included |
| `on` | string | `"output"` | `"output"` for completed lines or `"prompt"` for live prompt-area updates |
| `confirmed_only` | bool | false | With `on = "prompt"`, match only updates terminated by Telnet GA/EOR |
| `span` | table | — | Collect a multi-line message; see [Multi-line triggers](#multi-line-triggers) |

## Prompt triggers

Some MUD prompts, especially `Username:` and `Password:`, have no line
delimiter and no Telnet GA/EOR marker. Rune displays such text immediately,
but the client cannot know whether it is a prompt or merely part of an
ordinary line split across socket reads. Prompt triggers opt into that
ambiguous path:

```lua
rune.trigger.exact("Username: ", function()
    rune.send("example-user")
end, { on = "prompt", once = true })
```

The output and prompt channels are exclusive. A default trigger runs once
when a delimiter completes a line; its action and state never process
prompt-area updates. A prompt trigger sees unfinished snapshots and
GA/EOR-confirmed prompts, but not completed lines. One presentation-only
exception prevents hidden output from flashing while incomplete: Rune checks
declarative `gag = true` output patterns read-only against unconfirmed updates.
It does not run their actions, consume `once`, or change span state.

Unfinished snapshots are cumulative in the common case and can repeat as
text grows. The same text can also run twice—first unconfirmed, then confirmed—
if GA/EOR arrives in a later socket read. A prompt action should therefore be
idempotent, or use `once` for one-shot work such as login.
`ctx.prompt_confirmed` is `false` for ambiguous unfinished text and `true` for
GA/EOR. Use `confirmed_only = true` for Mudlet-style, protocol-confirmed
prompt matching:

```lua
rune.trigger.regex("^HP:(\\d+)/(\\d+)>", update_vitals,
    { on = "prompt", confirmed_only = true })
```

Before an outbound line is written, Rune discards the current unfinished
accumulator. This cleanly separates no-GA login prompts such as `Username:`
and `Password:`. The unavoidable trade-off is that sending during an ordinary
line split makes the later completed line start after that send. A direct Lua
send clears the matching prompt overlay without adding it to scrollback; a
user submission commits the overlay first.

## Multi-line triggers

Server output often spans lines — wrapped chat, score sheets, who
lists, quest logs. A `span` collects the block: the trigger's pattern
matches the first line as usual, following lines are appended, and
the action fires **once** with the whole thing.

```lua
-- Terminator-delimited: a wrapped message ending in a color reset
rune.trigger.regex("^(\\w+) tells you: (.+)$", function(matches, ctx)
    forward("[Tell] " .. matches[1] .. ": " .. ctx.text)
end, { name = "tells", span = { to = "\\x1b\\[0?m\\s*$", raw = true, max = 8 } })

-- Fixed-count: a block that is always the same number of lines
rune.trigger.starts("You have scored", parse_score, { span = { max = 4 } })
```

| Field | Type | Default | Description |
|---|---|---|---|
| `to` | string | — | Regex for the line that ends the span, inclusive. Validated at registration. Optional: without it the span always runs to `max`. |
| `raw` | bool | false | Match `to` against the raw line — needed when the terminator is an escape code (like a trailing color reset) that stripping removes. Independent of the trigger-level `raw`, which governs pattern matching. |
| `max` | number | 8 | Flush after this many lines, first line included. |

In the action, `ctx.text` is the message text — the pattern's last
capture (or the whole clean line for the literal modes), with each
continuation line appended, space-joined. `ctx.lines` holds the
collected [line objects](/reference/api/state-lines/), first line
first. `matches` are the first line's captures, as usual; a string
action substitutes them and is sent once, at completion.

Behavior:

- Lines display as they arrive, so the action's return value is
  ignored — a span cannot rewrite. `gag = true` is the exception: it
  hides every collected line as it arrives, first line included.
- Collected lines still run through other triggers and hooks. A span
  sees each line as this trigger would have — including rewrites from
  higher-priority triggers.
- Partial prompt-area updates never open, extend, or end spans. A nonempty
  tail terminated by GA/EOR, any user submission, or an outbound command that
  closes an active prompt-overlay epoch ends open spans. The outbound case
  includes commands sent by Lua, so no-GA login automation creates the same
  boundary as typing the response; an unrelated timer or output trigger send
  does not truncate a span.
  A GA/EOR marker with no current tail produces no text event and does not
  flush spans. Connecting, disconnecting, and `/reload` discard open spans
  without firing them.
- A confirmed prompt flushes spans before `prompt_update` hooks and prompt
  triggers run. This boundary transition is not controlled by hook priority.
- One open span per trigger: if the pattern matches again mid-span,
  the previous message fires and a new span starts.
- If the first line also matches `to`, the message is complete
  immediately — single-line messages work with no special casing.
- `once` removes the trigger after its first completed span.

## Managing

Standard registry management applies:
`rune.trigger.enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/triggers` lists everything;
`/test <line>` feeds a fake completed line through the output-trigger
pipeline. It does not exercise prompt triggers.

**Related:** [Triggers guide](/scripting/triggers/) ·
[rune.alias](/reference/api/alias/) · [rune.regex](/reference/api/regex/) ·
[rune.hooks](/reference/api/hooks/)
