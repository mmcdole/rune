---
title: rune.timer
description: Full signatures for one-shot and repeating timers — scheduling, remaining time, self-cancellation, and disabling callbacks.
---

Timers run actions after a delay or at a fixed interval. For a
task-oriented introduction, see [Timers](/scripting/timers/).

## Quick reference

```lua
rune.timer.after(seconds, action, opts?)   -- one-shot: fires once, then removes itself
rune.timer.every(seconds, action, opts?)   -- repeating: fires every interval
rune.timer.cancel(name)                    -- alias of rune.timer.remove
rune.timer.list()                          -- registrations with remaining seconds
```

Both constructors return a [handle](/reference/api/#handles) and accept
the [common options](/reference/api/#options) (`name`, `group`). Timer
handles additionally accept `h:cancel()` as an alias of `h:remove()`.

### rune.timer.every

```lua
rune.timer.every(seconds, action, opts?) -> handle
```

- `seconds` (number) — the interval, in seconds (fractions allowed).
- `action` (string | function) — a command string sent on each firing,
  or `function(ctx)`.
- `opts` (table, optional) — [common options](/reference/api/#options).

Scheduling is fixed-interval: the next firing is scheduled the moment
the previous one fires, regardless of how long the action takes to run.

```lua
rune.timer.every(60, "save", {name = "autosave"})
```

## Actions and self-cancellation

A string action is sent as a command. A function action receives a
context table (`ctx.name`, `ctx.group`, `ctx.type`); `ctx:remove()` is
how a repeating timer stops itself:

```lua
local ticks = 0
rune.timer.every(10, function(ctx)
    ticks = ticks + 1
    rune.send("look")
    if ticks >= 5 then ctx:remove() end
end)
```

## Disabling and enabling

`h:disable()` suppresses firing without unregistering; `h:enable()`
resumes. A repeating timer keeps its schedule while disabled and picks
it back up on re-enable.

:::caution
A **one-shot** whose moment passes while disabled is removed — its
wake-up is spent and re-enabling cannot revive it. Timers are also
Lua-registered state, so `/reload` clears them all; register timers in
your script so they come back on reload.
:::

## Managing

Standard registry management applies:
`rune.timer.get/enable/disable/remove(name)`, `.cancel(name)`, `.list()`,
`.count()`, `.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/timers` lists everything.

### rune.timer.list

```lua
rune.timer.list() -> { timer, ... }
```

Each entry includes:

| Field | Meaning |
|---|---|
| `seconds` | Configured delay or repeat interval, in seconds |
| `remaining` | Seconds until the next scheduled wake-up, including fractions; never negative |
| `mode` | `"after"` or `"every"` |
| `value` | The command string, or `"(function)"` for a callback |
| `name`, `enabled`, `group`, `source` | Registration metadata as described in [Registries](/reference/api/#managing) |

`remaining` is a snapshot from the Go timer scheduler. Repeating timers
count down to their next wake-up even while disabled, while their group
is disabled, or while an earlier callback is waiting to run. A one-shot
whose wake-up has already fired returns zero until Lua processes it and
removes the registration. Removed timers do not appear in the list.
This reports scheduling time, not a guarantee of when the callback will
execute on the client's event loop.

`/timers` shows the snapshot rounded to one decimal place, for example
`every 60.0s (23.4s left)`. It refreshes when you run the command again.

**Related:** [Timers guide](/scripting/timers/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.hooks](/reference/api/hooks/) ·
[rune.group](/reference/api/group/)
