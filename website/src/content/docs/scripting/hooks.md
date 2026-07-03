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
| `input` | the typed text | Return `false` to consume; other returns are ignored. The core handler at priority 100 dispatches commands and aliases. |
| `output` | a line object | `false` gags, a string rewrites. The core handler runs triggers at priority 100. |
| `prompt` | a line object | Same, for prompt fragments. |
| `echo` | the typed text | Like `output` but a plain string. The `> ` prefix is the core handler; replace it if you like. |

For `output`, `prompt`, and `echo`, rewrites chain: a handler returning a
string replaces the text for every subsequent handler, and `false` stops
the chain (gags the line or hides the echo). For `input`, only `false`
means anything. To rewrite input, use an
[alias](/rune/scripting/aliases/).

```lua
-- Timestamp every line, after triggers have run
rune.hooks.on("output", function(line)
    return rune.style.gray(os.date("[%H:%M] ")) .. line:raw()
end, { priority = 150 })
```

```lua
-- A panic key: swallow all input while active
local locked = false
rune.hooks.on("input", function(text)
    if locked and text ~= "/unlock" then return false end
end, { priority = 1 })
```

## Notification events

All handlers run and returns are ignored: `ready`, `connecting` (address),
`connected` (address), `disconnecting`, `disconnected`, `reloading`,
`reloaded`, `loaded` (path), `error` (message), `input_changed`,
`gmcp` (catch-all: `package, data, raw`), and `gmcp_enabled`.

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

| Option | Effect |
|---|---|
| `name` | Unique name. Registering the same name again replaces the old handler. |
| `group` | Adds the handler to a group. Toggle the set with `/group <name> on\|off`. |
| `priority` | Order among handlers for the event. Lower runs first (default 50). |

## Managing

Every constructor returns a handle with `:enable()`, `:disable()`, and
`:remove()`. By name: `rune.hooks.disable/enable/remove(name)`, and
`rune.hooks.list()` returns everything registered. In the client, `/hooks`
lists every handler, including the core's own, since the client registers
its behavior through the same API.

## Gotchas

- A handler that throws is skipped for that line and reported once; it
  cannot abort the chain. Three consecutive failures quarantine it. Fix the
  code, then re-enable it with `rune.hooks.enable(name)`.
- Handlers may register or remove hooks mid-dispatch safely; the chain
  iterates a snapshot.

**Related:** [Triggers](/rune/scripting/triggers/),
[GMCP](/rune/scripting/gmcp/),
[Hook events reference](/rune/reference/hook-events/)
