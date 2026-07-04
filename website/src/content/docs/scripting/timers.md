---
title: Timers
description: One-shot delays and fixed-interval repeats, with string or function actions and self-cancellation.
---

A timer runs an action once after a delay (`after`) or repeatedly at an
interval (`every`). The action is either a string sent as a command, or a
Lua function.

```lua
rune.timer.after(5, "stand")                          -- once, in 5 seconds
rune.timer.every(60, "save", { name = "autosave" })   -- every 60 seconds
```

```lua
-- Poll until done, then stop yourself
local tries = 0
rune.timer.every(2, function(ctx)
    tries = tries + 1
    rune.send("open gate")
    if tries >= 10 then ctx:remove() end
end)
```

## Creating

```lua
rune.timer.after(seconds, action, opts?)   -- fires once
rune.timer.every(seconds, action, opts?)   -- fires repeatedly
```

Strings go through `rune.send`, so `;` chaining and aliases apply. Use a
function when the timer needs a condition, state, or to cancel itself. The
function receives a `ctx` with `name`, `group`, `type`, and a
`ctx:remove()` method that removes the timer from inside its own callback.

`every` is fixed-interval: the next fire is scheduled the moment the
previous one fires, regardless of how long your callback takes.

## Options

Timers take the [common options](/scripting/model/#options) `name`
(same-name registration replaces) and `group`.

## Examples

A delayed sequence without callbacks inside callbacks:

```lua
rune.alias.exact("ritual", function()
    rune.send("kneel")
    rune.timer.after(2, "chant")
    rune.timer.after(4, "sacrifice goat")
end)
```

A keepalive that only runs while grouped:

```lua
rune.timer.every(300, "save", { name = "keepalive", group = "afk" })
-- /group afk on   when you walk away
```

## Managing

Every constructor returns a handle:

```lua
local h = rune.timer.every(60, "save", { name = "autosave" })
h:disable()  h:enable()  h:cancel()   -- :cancel() is an alias for :remove()
```

By name: `rune.timer.disable/enable/remove(name)` (`rune.timer.cancel` is
the same as `remove`) — the full management suite is in the
[API reference](/reference/api/#managing). In the client, `/timers` shows
every timer with its state, mode and interval, action, group, name, and
the `file:line` that registered it.

Timers are cleared on `/reload` and re-registered when your scripts load
again, which keeps reloads deterministic.

## Gotchas

- Timer callbacks run on the client's event loop under the same watchdog as
  everything else: a callback stuck in a loop is interrupted, and one that
  keeps erroring is [quarantined](/scripting/model/#quarantine).
- Disabling suppresses firing. A repeating timer keeps its schedule and
  resumes on re-enable. A one-shot whose moment passes while disabled is
  removed; its wake-up is spent, and re-enabling cannot revive it.
- Sub-second intervals work (`rune.timer.every(0.25, ...)`), but every fire
  crosses into Lua, so keep fast timers cheap.

**Related:** [rune.timer reference](/reference/api/timer/),
[Groups](/scripting/groups/),
[Hooks & Events](/scripting/hooks/)
