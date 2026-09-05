---
title: Hooks & Events
description: Inspect or change input and output, and respond to client events.
---

Hooks let a script inspect or change input and output, and respond to events
such as connecting or reloading. Rune also registers named hooks for triggers,
local echo styling, and history expansion; `/hooks` lists them. After all input
hooks finish, Rune handles the resulting text as normal command input or
verbatim input.

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
| `input` | submitted text, context | Before echo and history: a string, including `""`, replaces the text for later handlers, `false` cancels it, and any other value leaves it unchanged. `context.mode` is read-only and always `"command"` or `"verbatim"`. |
| `output` | a line object | Once per complete line. `false` gags, a string rewrites. The core handler runs output triggers at priority 100. |
| `prompt` | a line object, `confirmed` | Cumulative partial-line observation (`false`) or GA/EOR-confirmed prompt (`true`). The core runs prompt triggers at priority 100. |
| `echo` | one display-safe line of the final input | Like `output` but a plain string. Terminal controls have already been made visible. The `> ` prefix is the core handler; replace it if you like. |

For every data-flow event, rewrites chain: a handler returning a string
replaces the text for every subsequent handler, `nil` or another value passes
the current text through, and `false` stops the chain. For input, `false`
cancels the submission before local echo, history, and input processing.
Otherwise Rune uses the final rewrite for local echo and input processing,
and records it in history unless it is empty.

The input hook runs before the current submission enters history, so handlers
see only earlier accepted submissions. In verbatim mode `text` is the whole
draft and may contain line breaks. Existing handlers that accept only `text`
continue to work because Lua ignores the extra context argument.

The final command-mode replacement must remain valid command text: ordinary
game commands stay on one line, while local `/commands` may carry multiline,
tab-indented arguments. Terminal controls are rejected. For invalid text, Rune
reports an error and does not echo, save, or send that submission. To
deliberately send several physical lines, call `rune.send_raw` and return
`false` so Rune does not also process the original command. Verbatim-mode
handlers may rewrite the whole multiline draft.

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
-- Rewrites chain: later input hooks, echo, history, and command processing
-- all see "look".
rune.hooks.on("input", function(text)
    if text == "l" then return "look" end
end, { priority = 25 })
```

```lua
-- Handle one command by sending two physical lines yourself.
rune.hooks.on("input", function(text)
    if text == "combo" then
        rune.send_raw("kick\npunch")
        return false
    end
end, { priority = 25 })
```

All input handlers run in priority order, with lower numbers first. If none
returns `false`, Rune processes the final text after the last handler finishes.
In command mode that means slash commands, command separators, `#N` repeats,
and aliases. Verbatim input sends each line without any of that command
processing. Even a handler with a priority above 100 still runs before Rune
processes or sends the command.

The named core input hook `history-expansion` runs at priority 100. With the
default history character, it expands interactive command components such as
`!`, `!!`, and `!k`; see [Input & History](/interface/input/#history) for the
full behavior. If your game uses bang commands, choose another character or
disable expansion with `rune.config.set("history_character", "")`.

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

Every command you submit finishes the partial line before input hooks and any
echo, history, or command processing. This also applies to local slash
commands, canceled commands, and commands that cannot be sent because you are
disconnected. Commands sent by aliases, triggers, timers, and other scripts
finish the partial line only after the connection accepts them. Failed sends
and protocol traffic such as GMCP do not.

## Notification events

All handlers run and returns are ignored. The ones you'll reach for
most: `connected` (address), `disconnected`, and `ready` (after scripts load
during startup and reload). The full catalog — connection lifecycle, reload,
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

The named `history-expansion` input handler uses priority 100. Lower-priority
handlers run before expansion and higher-priority handlers see its result. All
continue to run unless an earlier input handler returns `false`; Rune processes
the final text only after they finish.

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
- A newly added handler starts with the next event. If a handler removes or
  disables another handler that has not run yet, the removed handler is skipped
  immediately.

**Related:** [rune.hooks reference](/reference/api/hooks/),
[Triggers](/scripting/triggers/),
[GMCP](/scripting/gmcp/)
