---
title: The Scripting Model
description: "Options, actions, handles, groups, and quarantine: the machinery every registration shares."
---

Everything you register with rune (aliases, triggers, timers, hooks, key
bindings, bars, GMCP handlers, slash commands) lives in the same kind of
registry and behaves the same way. This page describes that shared behavior
in one place, so the guide for each one doesn't repeat it.

If you haven't written a trigger or an alias yet, start with
[Triggers](/scripting/triggers/) and come back. This page makes more sense
once you have registered a few things.

## Options

Every creation function takes an optional `opts` table as its last argument.
Four fields work everywhere:

| Option | Type | Default | Description |
|---|---|---|---|
| `name` | string | none | Unique ID for management. |
| `group` | string | none | Group membership for batch enable/disable/remove. |
| `priority` | number | 50 | Execution order where multiple items can match (regex aliases, triggers, hooks). Lower runs first. |
| `once` | bool | false | Auto-remove after the first match (aliases, triggers). |

Naming an item does more than label it. Registering the same name again
replaces the old entry instead of adding a second one, which is what keeps
`/reload` from stacking a duplicate trigger every time you edit a script.
Name anything you expect to re-register.

Individual registries add their own options. Triggers take `gag` and `raw`,
for example. Each [API reference page](/reference/api/) lists its extras; the
four above work everywhere.

## String and function actions

Wherever an action is expected, a string is sent as a command and a
function runs your logic:

```lua
rune.alias.exact("n", "north")                 -- string: sent as-is
rune.trigger.contains("hungry", "eat bread")   -- string: sent on match

rune.alias.exact("heal", function(args, ctx)   -- function: full control
    rune.send("cast heal " .. (args ~= "" and args or "self"))
end)
```

Regex string actions substitute captures with `%1`, `%2`, …:

```lua
rune.alias.regex("^cmd\\s+(\\w+)\\s+(.+)", "command private %1 to %2")
```

## The context object

Function actions receive a context table as their last argument:

| Field | Description |
|---|---|
| `ctx.name` | The item's name, if set |
| `ctx.group` | The item's group, if set |
| `ctx.type` | `"alias"`, `"trigger"`, `"timer"`, or `"hook"` |
| `ctx.line` | The original line (a [line object](/reference/api/state-lines/) for triggers) |
| `ctx.args` | Text after the matched phrase (exact aliases) |
| `ctx.matches` | Capture array (regex matches) |
| `ctx:remove()` | Remove this item from inside its own callback |

`ctx:remove()` is how a timer stops itself:

```lua
rune.timer.every(10, function(ctx)
    if done then ctx:remove() end
end)
```

## Managing without handles

Give an item a `name` and you can reach it from anywhere, which is how you
will manage things most of the time:

```lua
rune.trigger.contains("spam", nil, { gag = true, name = "spam-gag" })
rune.trigger.disable("spam-gag")
```

Every registry exposes the same functions, taking the item name: `enable`,
`disable`, `remove`, plus `list()`, `count()`, `clear()`, and
`remove_group(group)`. The full contract is in the
[API reference](/reference/api/#managing).

## Handles

For the times you would rather hold a reference than name a thing, every
creation function also returns a handle:

```lua
local h = rune.trigger.contains("spam", nil, {gag = true, name = "spam-gag"})

h:disable()  -- stop firing, stay registered
h:enable()   -- resume
h:remove()   -- unregister
h:name()     -- "spam-gag"
h:group()    -- nil (no group set)
```

Methods chain, which is handy for registering something already switched off:

```lua
rune.trigger.contains("Weather:", nil, {gag = true}):disable()
```

## Groups

Items have two independent enable switches: their own state
(`h:disable()`) and their group's master switch
(`rune.group.disable("combat")`). An item fires only when **both** are
enabled, and re-enabling a group preserves each item's individual state.
See [Groups](/scripting/groups/) for patterns.

## Quarantine

:::caution
A callback that throws **three times in a row** is quarantined: rune
disables that one item and prints a notice, so a broken script can't
flood your screen or wedge the input pipeline. Everything else keeps
running.
:::

To recover, fix the error and re-enable the item with `h:enable()`,
`rune.trigger.enable("name")`, or just `/reload` (re-registering
resets the failure count). One successful run also clears the count.

## Source attribution

Every registration records the registering script's `file:line`. It shows up
in error messages and in the listing commands (`/aliases`, `/triggers`,
`/timers`, `/hooks`, `/binds`, `/bars`), so you can always tell which script
owns an item.

**Related:** [API reference overview](/reference/api/) ·
[Triggers](/scripting/triggers/) · [Aliases](/scripting/aliases/) ·
[Groups](/scripting/groups/)
