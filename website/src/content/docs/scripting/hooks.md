---
title: Hooks & Events
description: The event pipeline under everything else. Intercept input, output, prompts, and system events.
---

Hooks are the lowest-level scripting surface: every line in or out of the
client flows through them, and the core's own behavior (trigger dispatch,
echo styling, command handling) is implemented as hook handlers you can see
in `/hooks`.

```lua
rune.hooks.on(event, handler, opts?)
```

Unlike triggers, aliases, and timers, a hook handler is always a Lua
function; there is no string form. What the function receives and what its
return value means depend on the event.

## Data-flow events

Handlers run in priority order:

| Event | Handler receives | Notes |
|---|---|---|
| `input` | submitted text, context | Return `false` to consume; other returns are ignored. `context.mode` is read-only and always `"command"` or `"verbatim"`. |
| `output` | a line object | Once per completed line. `false` gags, a string rewrites. The core handler runs output triggers at priority 100. |
| `prompt_update` | a line object, `prompt_confirmed` | Live unfinished text (`false`) or a GA/EOR-confirmed prompt (`true`). The core runs opted-in prompt triggers at priority 100. |
| `echo` | one physical line of typed text | Like `output` but a plain string. The `> ` prefix is the core handler; replace it if you like. |

For `output`, `prompt_update`, and `echo`, rewrites chain: a handler returning a
string replaces the text for every subsequent handler, and `false` stops the
chain (gags the line or hides the echo). For `input`, only `false` means
anything. The hook fires once per submission: in verbatim mode `text` is the
whole draft and may contain LF characters. Existing handlers that accept only
`text` continue to work because Lua ignores extra arguments. To rewrite normal
command input, use an [alias](/scripting/aliases/).

An unconfirmed `prompt_update` is not a claim that the text is a prompt. With
no line delimiter, GA, or EOR, Rune cannot tell `Username:` from the first half
of a longer line. Rune displays the unfinished text immediately and reports
that uncertainty to Lua. Updates can repeat as the text grows; if it later
ends as a normal line, the complete text also fires once through `output`,
unless an outbound command separated the unfinished text first. If GA/EOR
arrives separately, identical text can fire again with `prompt_confirmed`
changing from `false` to `true`. A marker with no current text does not fire
`prompt_update`.

If more server text follows, Rune commits a confirmed prompt before the new
record, even when that record first arrives as a partial. It replaces an
unconfirmed preview without committing it separately. A confirmed prompt also
flushes open multi-line triggers before `prompt_update` handlers run.

```lua
-- Repaint from the latest snapshot; do not accumulate per update.
rune.hooks.on("prompt_update", function(line, prompt_confirmed)
    current_prompt = line:clean()
end)
```

```lua
-- Timestamp every line, after triggers have run
rune.hooks.on("output", function(line)
    return rune.style.gray(os.date("[%H:%M] ")) .. line:raw()
end, { priority = 150 })
```

```lua
-- A panic key: swallow all input while active
local locked = false
rune.hooks.on("input", function(text, context)
    if locked and text ~= "/unlock" then return false end
    if context.mode == "verbatim" then
        rune.echo("Verbatim input bypasses aliases and slash commands")
    end
end, { priority = 1 })
```

The core handler at priority 100 applies aliases, `;` separators, `#N`
repeats, and slash commands when `context.mode == "command"`. For
`"verbatim"`, it splits only on LF and sends every physical line as data.

## Notification events

All handlers run and returns are ignored. The ones you'll reach for
most: `connected` (address), `disconnected`, and `ready` (boot and
reload complete). The full catalog — connection lifecycle, reload,
errors, GMCP — is in the
[rune.hooks reference](/reference/api/hooks/#notification-events).

```lua
rune.hooks.on("connected", function(addr)
    rune.send("Ragnar")  -- or see the auto-login cookbook recipe
end)
```

## Priorities in practice

The core's data-flow handlers sit at priority 100. Run before them
(priority below 100) to intercept raw input and output. Run after them
(priority above 100) to see the post-trigger result; that is where the
session logger lives (priority 200, named `log-output`). One exception: the
core `input` handler always consumes, so `input` handlers above priority
100 never run.

## Options

Hooks take the [common options](/scripting/model/#options) `name`,
`group`, and `priority` (order among handlers for the event, lower
first, default 50).

## Managing

Every constructor returns a handle with `:enable()`, `:disable()`, and
`:remove()`. By name: `rune.hooks.disable/enable/remove(name)` — the full
management suite is in the [API reference](/reference/api/#managing). In
the client, `/hooks` lists every handler, including the core's own, since
the client registers its behavior through the same API.

## Gotchas

- A handler that throws is skipped for that line and reported once; it
  cannot abort the chain. Three consecutive failures
  [quarantine](/scripting/model/#quarantine) it.
- Handlers may register or remove hooks mid-dispatch safely; the chain
  iterates a snapshot.

**Related:** [rune.hooks reference](/reference/api/hooks/),
[Triggers](/scripting/triggers/),
[GMCP](/scripting/gmcp/)
