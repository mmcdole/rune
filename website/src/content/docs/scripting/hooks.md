---
title: Hooks & Events
description: The event pipeline under everything else. Intercept input, output, prompts, and system events.
---

Hooks are the lowest-level scripting surface: every line in or out of the
client flows through them. Core policies such as trigger dispatch, echo
styling, and interactive history expansion are handlers you can see in
`/hooks`; final command or verbatim routing is an internal step after input
hooks finish.

```lua
rune.hooks.on(event, handler, opts?)
```

Reach for a [trigger](/scripting/triggers/) when you want to match particular
text. Reach for a hook when you want every line regardless of content, to
timestamp or log or mirror it, or when what you care about isn't text at all,
such as connecting, reloading, or a GMCP message.

Unlike triggers, aliases, and timers, a hook handler is always a Lua
function; there is no string form. What the function receives and what its
return value means depend on the event.

## Data-flow events

Handlers run in priority order:

| Event | Handler receives | Notes |
|---|---|---|
| `input` | submitted text, context | Precommit: strings rewrite and chain, `false` consumes, and other values pass through. `context.mode` is read-only and always `"command"` or `"verbatim"`. |
| `output` | a line object | Once per complete line. `false` gags, a string rewrites. The core handler runs output triggers at priority 100. |
| `prompt` | a line object, `confirmed` | Cumulative partial-line observation (`false`) or GA/EOR-confirmed prompt (`true`). The core runs prompt triggers at priority 100. |
| `echo` | one physical line of effective input | Like `output` but a plain string. The `> ` prefix is the core handler; replace it if you like. |

For every data-flow event, rewrites chain: a handler returning a string
replaces the text for every subsequent handler, `nil` or another value passes
the current text through, and `false` stops the chain. For input, `false`
consumes before local echo, history, and dispatch. Otherwise the final rewrite
is the text Rune records, echoes, and routes.

The input hook runs before the current submission enters history, so handlers
see only earlier accepted submissions. In verbatim mode `text` is the whole
draft and may contain line breaks. Existing handlers that accept only `text`
continue to work because Lua ignores the extra context argument.

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

```lua
-- Rewrites chain: later input hooks, echo, history, and dispatch see "look".
rune.hooks.on("input", function(text)
    if text == "l" then return "look" end
end, { priority = 25 })
```

After every input hook runs, Rune internally applies aliases, delimiters,
`#N` repeats, or slash commands when `context.mode == "command"`. Verbatim
input instead recognizes LF, CRLF, and bare CR line breaks and bypasses command
processing. Routing is not a hook, so input handlers have no priority cutoff.

The named core input hook `history-expansion` runs at priority 100. It expands
interactive command components such as `!`, `!!`, and `!k`; see
[Input & History](/interface/input/#history) for the full behavior. Remove it
with `rune.hooks.remove("history-expansion")` if your game uses bang commands.

## Prompt semantics

The `prompt` hook drives the prompt overlay; its name does not mean every value
is a prompt. A partial line may be `Username:` or the first half of a longer
line, and rune cannot tell which until the server finishes it. So the same text
can reach you more than once: the observation repeats as the line grows, and if
GA/EOR arrives separately it runs again with `confirmed` changing from `false`
to `true`. Write handlers that replace state rather than accumulate it:

```lua
-- Replace state on each observation; partial lines may repeat.
local current_prompt = ""
rune.hooks.on("prompt", function(line)
    current_prompt = line:clean()
end)
```

If the text turns out to be an ordinary line, it arrives once more through
`output` as a complete line, and that complete line replaces its partial line.
An empty prompt boundary does not fire `prompt`. Rune uses no timer or prompt
pattern to infer one. A confirmed prompt is committed before later server text
and closes open spans before `prompt` handlers run, so span actions have
already fired by the time your handler sees the prompt.

Every user submission finishes the partial line before input hooks and any
echo, history, or routing, including local slash commands and submissions that
are consumed, disconnected, or eventually fail to send. Programmatic game
lines from aliases, triggers, timers, and other callbacks finish it only after
the connection accepts the send. Failed programmatic sends and protocol
traffic such as GMCP do not.

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

The core's output, prompt, and echo handlers sit at priority 100. Run before
them to see the pre-trigger/pre-style value, or after them to see their result;
the session logger, for example, is `log-output` at priority 200.

Input uses priority 100 for the named `history-expansion` rewrite, not for
routing. Lower-priority hooks run before expansion and higher-priority hooks
see its result. All continue to run unless an earlier input handler returns
`false`.

## Options

Hooks take the [common options](/scripting/model/#options) `name`,
`group`, and `priority` (order among handlers for the event, lower
first, default 50).

## Managing

Every constructor returns a handle with `:enable()`, `:disable()`, and
`:remove()`. By name: `rune.hooks.disable/enable/remove(name)`. The full
list is in the [API reference](/reference/api/#managing). In
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
