---
title: Timers
description: Run commands after a delay or at regular intervals, and check how much time is left.
---

A timer runs an action once after a delay (`after`) or repeatedly at an
interval (`every`). The action is either a string sent as a command, or a
Lua function.

```lua
rune.timer.after(5, "stand")                          -- once, in 5 seconds
rune.timer.every(60, "save", { name = "autosave" })   -- every 60 seconds
```

```lua
-- Try five times, then stop
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

For a repeating timer, the countdown to the next repeat starts when the
timer becomes due, without waiting for its action to finish.

## Options

Timers take the [common options](/scripting/model/#options) `name` and
`group`. Creating a timer with an existing name replaces the old timer.

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

Creating a timer returns a handle you can use to manage it:

```lua
local h = rune.timer.every(60, "save", { name = "autosave" })
h:disable()  h:enable()  h:cancel()   -- :cancel() is an alias for :remove()
```

By name: `rune.timer.disable/enable/remove(name)` (`rune.timer.cancel` is
the same as `remove`). The full list is in the
[API reference](/reference/api/#managing). In the client, `/timers` shows
every timer with its state, mode and interval, time remaining, action,
group, name, and the `file:line` that registered it. For example,
`every 60.0s (23.4s left)` means the next scheduled firing is about 23.4
seconds away. These values are from when you run `/timers`; run it again
to see an updated countdown.

## Time remaining

To check how many seconds are left, call `:remaining()` on the timer:

```lua
local h = rune.timer.after(30, "stand")
local seconds = h:remaining()
```

The result includes fractions of a second. It is `nil` when the timer has
finished, been cancelled, or been removed.

You can also find a timer by name:

```lua
local timer = rune.timer.get("autosave")
local seconds = timer and timer:remaining()
```

The `timer and` check handles the case where no timer has that name.
If you create another timer with the same name, use `get()` again to
find the new one. The old handle still refers to the old timer.

To inspect all timers, use `rune.timer.list()`:

```lua
for _, timer in ipairs(rune.timer.list()) do
    rune.echo(string.format("%s: %.1fs left", timer.name, timer.remaining))
end
```

Disabled timers and timers in disabled groups keep counting down; their
actions are skipped while disabled. A one-shot that is due but still
waiting to run shows `0.0s left`.

`/reload` clears all timers. Create them in your script if they should
start again when you reload.

## Gotchas

- If Rune is busy, a timer's action may run later than the countdown suggests.
- Rune interrupts timer functions that run too long and
  [automatically disables](/scripting/model/#quarantine) timers whose
  actions keep producing errors.
- Disabling a repeating timer skips its actions without pausing or restarting
  its countdown. A one-shot that becomes due while disabled is removed
  without running; create a new timer if you still need the action.
- Sub-second intervals work (`rune.timer.every(0.25, ...)`). Keep frequently
  repeated actions short so they do not slow down the client.

**Related:** [rune.timer reference](/reference/api/timer/),
[Groups](/scripting/groups/),
[Hooks & Events](/scripting/hooks/)
