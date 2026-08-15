---
title: rune.hooks
description: Event handlers with priority ordering, plus the full catalog of data-flow and notification events.
---

Hooks attach handlers to client events — input, output, connection
lifecycle, GMCP, and more. For a task-oriented introduction, see
[Hooks & Events](/scripting/hooks/).

## Quick reference

```lua
rune.hooks.on(event, handler, opts?)   -- attach a handler to an event
rune.hooks.enable(name)                -- re-enable a named handler
rune.hooks.disable(name)               -- disable without unregistering
rune.hooks.remove(name)                -- unregister a named handler
rune.hooks.list()                      -- all handlers with event, priority, source
rune.hooks.clear(event?)               -- remove handlers for one event, or all
rune.hooks.has(event)                  -- true if the event has handlers
rune.hooks.count(event?)               -- handlers for one event, or total
rune.hooks.remove_group(group)         -- remove all handlers in a group
```

`on` returns a [handle](/reference/api/#handles) and accepts the
[common options](/reference/api/#options) (`name`, `group`, `priority`).

### rune.hooks.on

```lua
rune.hooks.on(event, handler, opts?) -> handle
```

- `event` (string) — an event name from the tables below.
- `handler` (function) — receives the event's arguments; return values
  matter only for data-flow events.
- `opts` (table, optional) — [common options](/reference/api/#options).
  `priority` defaults to 50; lower runs first.

```lua
rune.hooks.on("connected", function(addr)
    rune.send("look")
end, {name = "auto-look"})
```

## Data-flow events

Handlers run in priority order (lower first, default 50).

For `output`, `prompt`, `echo`, and `input`, a string replaces the text for
subsequent handlers, so rewrites chain in priority order. `nil` and other
values pass the current text through. `false` stops the chain: it gags output
or a prompt, hides an echo, or consumes an input submission.

Input is a precommit hook. A consumed submission is not locally echoed, added
to history, or dispatched. Otherwise its final effective text is recorded,
echoed, and routed after every input handler has run.

| Event | Handler receives | Fired |
|---|---|---|
| `input` | submitted text, context | At precommit, before local echo, history, and command or verbatim routing |
| `output` | line object (`:raw()`, `:clean()`) | Once for every complete server line |
| `prompt` | line object, `confirmed` boolean | On cumulative partial-line observations and GA/EOR-confirmed prompts |
| `echo` | effective input text | On each physical line after input rewrites; skipped while the server has echo suppressed (passwords) |

`prompt` drives the prompt overlay. With `confirmed = false`, the value is a
partial line. It may be `Username:` or only the start of an ordinary line, and
it may repeat as it grows. A line delimiter later sends the complete line
through `output`. With `confirmed = true`, a GA/EOR prompt boundary confirmed
the text as a prompt. If the boundary arrives in a later batch, the hook may
receive the same text first as partial and then as confirmed. An empty prompt
boundary does not fire the hook. Rune never uses a timer or prompt pattern to
confirm a partial line.

Finishing a partial line means one thing throughout Rune: the prompt overlay
is committed to scrollback and open trigger spans close. Every user submission
finishes the partial line before input hooks and any echo, history, or routing,
even for a local slash command, a consumed submission, a disconnected
submission, or one whose eventual send fails. Separately, a programmatic game
line from an alias, trigger, timer, or other callback finishes it only after
the connection accepts the send. A failed programmatic send and protocol
traffic such as GMCP do not.

If the partial text was really the start of a fragmented ordinary line,
sending commits the visible prefix; the rest arrives through `output` as a new
line. This is the trade-off for immediate partial-line display without a timer
or prompt pattern.

```lua
rune.hooks.on("prompt", function(line, confirmed)
    if confirmed then
        rune.echo("Server-confirmed prompt: " .. line:clean())
    end
end)
```

Handlers for partial lines should be safe to repeat. A confirmed prompt closes
open spans before `prompt` handlers run. Finishing a partial line on submission
or accepted send commits its latest processed overlay without calling the
`prompt` hook again.

Every `input` handler receives `(text, context)`. The context is read-only, and
`context.mode` is always `"command"` or `"verbatim"`. Each handler sees the
text returned by the preceding handler:

```lua
rune.hooks.on("input", function(text, context)
    if context.mode == "verbatim" then
        -- The handler sees the whole submission once, line breaks included.
        rune.echo("Sending verbatim block (" .. #text .. " bytes)")
    end
    return text:gsub("^l$", "look")
end, { priority = 10 })
```

Input hooks run before the current submission enters history, so a handler
that calls `rune.history.get()` sees only earlier accepted submissions. A
string rewrite controls the later local echo, history entry, and dispatch.
Existing one-argument handlers remain valid because Lua ignores the context
argument. Echo hooks run after the effective submission is recorded and can
therefore observe its history entry.

Command mode applies Rune aliases, separators, `#N` repeats, and slash
commands after the input-hook chain. Verbatim mode instead splits LF, CRLF,
and bare CR into physical lines without command processing. Routing is an
internal step, not an input hook, so there is no priority cutoff: handlers at
any priority run unless an earlier handler returns `false`.

The named core input hook `history-expansion` runs at priority 100. On
interactive command submissions, complete delimiter-separated components
matching `!`, `!!`, or `!prefix` expand from the prior command history. It
skips slash-command entries, verbatim entries, and entries that contain an
unresolved bang designator. If any designator has no match, it warns and
consumes the entire submission. A submission beginning with `/` bypasses this
hook, leaving local-command arguments literal. Disable it with:

```lua
rune.hooks.remove("history-expansion")
```

History expansion is not command dispatch: `rune.send("!")` sends a literal
`!`, and verbatim submissions never expand it. Hooks below priority 100 see
the text before the core expansion; hooks above 100 see its result.

For `output`, `prompt`, and `echo`, the core handlers remain at priority 100:
trigger processing on `output`/`prompt` and the `> ` styling on `echo`.
Register below 100 to run before them, or above 100 to see their results
(post-trigger rewrites; gagged lines never reach you).

## Notification events

All handlers run; return values are ignored.

| Event | Args | Fired |
|---|---|---|
| `ready` | none | Boot complete, after user scripts load (fires again on `/reload`) |
| `connecting` | address | Dial started |
| `connected` | address | Connection established |
| `disconnecting` | none | Disconnect requested |
| `disconnected` | none | Connection closed |
| `reloading` / `reloaded` | none | Around `/reload` (order: `reloading`, `ready`, `reloaded`) |
| `loaded` | path | After `/load` or `rune.load` loads a file (not for startup auto-load) |
| `error` | message | On reported errors |
| `input_changed` | text | As the input line changes while typing |
| `window_size_changed` | width, height | On the first reported terminal size and every resize; `rune.state.width`/`height` already hold the new values |
| `gmcp` | package, data, raw JSON | On every GMCP message, before package-specific `rune.gmcp.on` handlers |
| `gmcp_enabled` | none | GMCP negotiated; the core handler sends `Core.Hello` |

`window_size_changed` receives the terminal columns and rows as numbers,
so a script can switch layouts at its own breakpoint:

```lua
rune.hooks.on("window_size_changed", function(width, height)
    if width < 80 then
        rune.ui.layout({ bottom = { "input" } })
    else
        rune.ui.layout({ bottom = { "input", "status" } })
    end
end)
```

`/reload` does not fire it; apply your initial layout from
`rune.state.width` when your script loads, and use the hook for later
changes.

## Named core handlers

Handlers the core registers under stable names, so you can disable or
replace them: `log-output`, `log-echo` (logging policy, priority 200),
`gmcp-hello` (the GMCP handshake), `gmcp-reset`, `first-run-welcome`,
`history-expansion` (interactive `!` expansion, priority 100), and
`_completion_cache` / `_completion_input` (tab-completion word harvesting,
priority 200).

## Managing

Standard registry management applies:
`rune.hooks.get/enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/hooks` lists everything.

**Related:** [Hooks & Events guide](/scripting/hooks/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.gmcp](/reference/api/gmcp/)
