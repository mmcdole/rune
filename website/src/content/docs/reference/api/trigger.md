---
title: rune.trigger
description: Full signatures for reacting to server text — match modes, actions, gag and rewrite.
---

Triggers match server text and run actions. For a
task-oriented introduction, see [Triggers](/scripting/triggers/).

## Quick reference

```lua
rune.trigger.exact(line, action, opts?)      -- whole line matches exactly
rune.trigger.starts(prefix, action, opts?)   -- line starts with prefix
rune.trigger.contains(text, action, opts?)   -- line contains text
rune.trigger.regex(pattern, action, opts?)   -- Go regexp, with captures
```

All constructors return a [handle](/reference/api/#handles) and accept
the [common options](/reference/api/#options) plus `gag`, `raw`, `on`, and
[`span`](#multi-line-triggers).

Triggers match complete lines by default. Set `on = "prompt"` to match
partial lines and GA/EOR prompts instead.

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
with `:raw()` and `:clean()`. The return value controls the line:

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
| `on` | string | `"output"` | `"output"` for complete lines or `"prompt"` for partial lines and GA/EOR prompts |
| `span` | table | — | Collect a multi-line output message; cannot be combined with `on = "prompt"` |

## Prompt triggers

Most triggers use `on = "output"` and run only after a complete line arrives.
Some MUDs send prompts without a newline, so Rune also exposes the partial
line through `on = "prompt"`:

```lua
rune.trigger.contains("Username:", "example-user",
    { on = "prompt", once = true })
```

Each trigger observes exactly one of the two streams:

| Setting | Text it observes |
|---|---|
| `on = "output"` | Complete server lines. This is the default. |
| `on = "prompt"` | Partial lines and prompts confirmed by Telnet GA/EOR. |

The same text can pass through both streams over its lifetime. Rune does not
wait on a timer or use a pattern to decide whether a partial line is a
prompt: prompt triggers observe it as it grows (observations can repeat), and
if CR/LF later completes it, the completed line runs through output triggers.
A prompt confirmed by GA/EOR stays with the prompt triggers, as does a
partial line finished by a submission or an accepted send: its latest
observation is committed without ever reaching output triggers. Actions
should be safe to repeat; use `once` for one-shot work such as login.

Prompt triggers can rewrite or gag the displayed prompt; rewrites chain, so
later prompt triggers match the rewritten text. They cannot use `span`, and
partial-line observations never alter an open span. Function actions receive
`ctx.confirmed`, which is `true` when GA/EOR confirmed the prompt.

Rune finishes the partial line before processing user-submitted input. A game
command sent by Lua finishes it only when the connection accepts the send.
Failed sends and protocol traffic such as GMCP leave it open. If the partial
text was part of an ordinary fragmented line, finishing it commits the visible
prefix and later data starts a new line.

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
- Partial-line prompt observations leave spans open. A confirmed prompt
  closes them before prompt hooks and triggers run.
- Finishing the partial line through submission or an accepted game send also
  closes spans, without running prompt triggers again. Empty prompt
  boundaries, failed sends, protocol traffic, and sends with no active prompt
  overlay do not close spans. A prompt a handler gagged to empty text still
  counts as active, so a later send closes spans even though nothing is
  visible.
- Connect and disconnect discard spans without firing. `/reload` is itself a
  submission, so it first finishes the partial line like any other
  submission, then discards any spans still owned by the old Lua VM.
- One open span per trigger: if the pattern matches again mid-span,
  the previous message fires and a new span starts.
- If the first line also matches `to`, the message is complete
  immediately — single-line messages work with no special casing.
- `once` removes the trigger after its first completed span.

## Managing

Standard registry management applies:
`rune.trigger.get/enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/triggers` lists everything;
`/test <line>` feeds a fake complete line through the output-trigger
pipeline. It does not exercise prompt triggers.

**Related:** [Triggers guide](/scripting/triggers/) ·
[rune.alias](/reference/api/alias/) · [rune.regex](/reference/api/regex/) ·
[rune.hooks](/reference/api/hooks/)
