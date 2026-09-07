---
title: rune.timer
description: Create timers, check time remaining, and enable, disable, or cancel their actions.
---

Timers run actions after a delay or at a fixed interval. For a
task-oriented introduction, see [Timers](/scripting/timers/).

## Quick reference

```lua
rune.timer.after(seconds, action, opts?)   -- one-shot: fires once, then removes itself
rune.timer.every(seconds, action, opts?)   -- repeating: fires every interval
rune.timer.cancel(name)                    -- alias of rune.timer.remove
rune.timer.list()                          -- all timers with remaining seconds
```

Both functions return a [handle](/reference/api/#handles) and accept
the [common options](/reference/api/#options) (`name`, `group`). Timer
handles additionally accept `h:cancel()` as an alias of `h:remove()`,
and `h:remaining()` to read the time left.

### rune.timer.every

```lua
rune.timer.every(seconds, action, opts?) -> handle
```

- `seconds` (number) — the interval, in seconds (fractions allowed).
- `action` (string | function) — a command string sent on each firing,
  or `function(ctx)`.
- `opts` (table, optional) — [common options](/reference/api/#options).

The countdown to the next repeat starts when the timer becomes due,
without waiting for its action to finish.

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

`h:disable()` stops the timer's action from running; `h:enable()` allows
it to run again. Disabling a repeating timer does not pause or restart
its countdown.

:::caution
A **one-shot** that becomes due while disabled is removed without running
its action. Enabling it afterward will not bring it back; create a new
timer instead. `/reload` clears all timers, so create them in your script
if they should start again when you reload.
:::

## Managing

Manage timers by name:
`rune.timer.get/enable/disable/remove(name)`, `.cancel(name)`, `.list()`,
`.count()`, `.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/timers` lists everything.

### handle:remaining

```lua
h:remaining() -> number | nil
```

Returns the number of seconds until this timer is next due, including
fractions; never negative. Reading the time left does not pause or restart
the timer. It works for both named and unnamed timers.

```lua
local h = rune.timer.after(30, "stand")
local seconds = h:remaining()

local autosave = rune.timer.get("autosave")
local left = autosave and autosave:remaining()
```

Returns `nil` when the timer has finished, been cancelled, or been removed.
Disabled timers and timers in disabled groups keep counting down. A
one-shot that is due but still waiting to run returns `0`. If Rune is busy,
the action may run later than the countdown suggests.

Creating another timer with the same name replaces the old timer, whose
handle then returns `nil`. Use `rune.timer.get(name)` to get the new timer.

Use `h:remaining()` to check one timer, or `rune.timer.list()` below to
check all timers.

### rune.timer.list

```lua
rune.timer.list() -> { timer, ... }
```

Each entry includes:

| Field | Meaning |
|---|---|
| `seconds` | Configured delay or repeat interval, in seconds |
| `remaining` | Seconds until the timer is next due, including fractions; never negative |
| `mode` | `"after"` or `"every"` |
| `value` | The command string, or `"(function)"` for a callback |
| `name`, `enabled`, `group`, `source` | Timer details as described in [Registries](/reference/api/#managing) |

`remaining` is the time left when you call `list()`. Call it again for
updated values. Repeating timers count down to the next repeat even when
they or their group are disabled, or an earlier action is waiting to run.
A one-shot that is due but still waiting to run shows zero. Finished and
cancelled timers do not appear in the list. If Rune is busy, an action may
run later than the countdown suggests.

`/timers` shows the time left rounded to one decimal place, for example
`every 60.0s (23.4s left)`. It refreshes when you run the command again.

**Related:** [Timers guide](/scripting/timers/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.hooks](/reference/api/hooks/) ·
[rune.group](/reference/api/group/)
