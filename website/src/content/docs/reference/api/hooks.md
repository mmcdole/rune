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

For `output`, `prompt`, and `echo`: `nil` passes through, a string
replaces the text for subsequent handlers (rewrites chain), and `false`
stops the chain (gag or hide).

For `input`: `false` consumes the submission; other return values are ignored.
Input handlers cannot rewrite by returning a string. Use `rune.input.set`, or
call `rune.send`/`rune.send_raw` yourself and return `false`.

| Event | Handler receives | Fired |
|---|---|---|
| `input` | submitted text, context | Once per submission, before command or verbatim routing |
| `output` | line object (`:raw()`, `:clean()`) | On every complete server line |
| `prompt` | line object | On prompt fragments (no newline, or GA/EOR terminated) |
| `echo` | typed text | On each physical line of local echo; skipped while the server has echo suppressed (passwords) |

Every `input` handler receives `(text, context)`. The context is read-only, and
`context.mode` is always `"command"` or `"verbatim"`:

```lua
rune.hooks.on("input", function(text, context)
    if context.mode == "verbatim" then
        -- text is the whole submission and may contain LF characters
        local _, breaks = text:gsub("\n", "")
        rune.echo("Sending " .. tostring(breaks + 1) .. " lines")
    end
end, { priority = 10 })
```

Command mode applies Rune aliases, separators, repeats, and slash commands in
the core handler. Verbatim mode still passes through custom `input` handlers
once, but the core treats only LF as a physical-line boundary and bypasses all
command interpretation. Existing one-argument handlers remain valid because
Lua ignores extra arguments.

The core registers its own handlers at priority 100: command or verbatim
routing on `input`, trigger processing on `output`/`prompt`,
the `> ` styling on `echo`. For `output`/`prompt`/`echo`, register
below 100 to run before the core, or above 100 to see its results
(post-trigger rewrites; gagged lines never reach you).

:::caution
The core `input` handler always returns `false`, so `input` handlers
must register with a priority **below 100** to run at all.
:::

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
| `gmcp` | package, data, raw JSON | On every GMCP message, before package-specific `rune.gmcp.on` handlers |
| `gmcp_enabled` | none | GMCP negotiated; the core handler sends `Core.Hello` |

## Named core handlers

Handlers the core registers under stable names, so you can disable or
replace them: `log-output`, `log-echo` (logging policy, priority 200),
`gmcp-hello` (the GMCP handshake), `gmcp-reset`, `first-run-welcome`,
and `_completion_cache` / `_completion_input` (tab-completion word
harvesting, priority 200).

## Managing

Standard registry management applies:
`rune.hooks.enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/hooks` lists everything.

**Related:** [Hooks & Events guide](/scripting/hooks/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.gmcp](/reference/api/gmcp/)
