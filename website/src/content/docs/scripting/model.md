---
title: The Scripting Model
description: Handles, options, groups, priorities, and quarantine — the machinery every registration shares.
---

Everything you register with rune — aliases, triggers, timers, hooks, key
bindings, bars, GMCP handlers, slash commands — lives in the same kind of
registry and behaves the same way. Learn the model once and every other
page gets shorter.

## Handles

Every creation function returns a handle:

```lua
local h = rune.trigger.contains("spam", nil, {gag = true, name = "spam-gag"})

h:disable()  -- stop firing, stay registered
h:enable()   -- resume
h:remove()   -- unregister
h:name()     -- "spam-gag"
h:group()    -- nil (no group set)
```

Methods chain:

```lua
rune.trigger.contains("Weather:", nil, {gag = true}):disable()
```

You rarely need to keep the handle. Name the item instead and manage it
by name from anywhere: `rune.trigger.disable("spam-gag")`.

## Options

Every creation function takes an optional `opts` table with the same
core fields:

| Option | Type | Default | Description |
|---|---|---|---|
| `name` | string | none | Unique ID for management. Registering the same name again **replaces** the old entry (upsert) — reloading a script never stacks duplicates. |
| `group` | string | none | Group membership for batch enable/disable/remove. |
| `priority` | number | 50 | Execution order where multiple items can match (aliases, triggers, hooks). Lower runs first. |
| `once` | bool | false | Auto-remove after the first match (aliases, triggers). |

Individual registries add their own options — triggers take `gag` and
`raw`, for example. Each [API reference page](/reference/api/) lists its
extras; the four above work everywhere.

## Groups

Items have two independent enable switches: their own state
(`h:disable()`) and their group's master switch
(`rune.group.disable("combat")`). An item fires only when **both** are
enabled, and re-enabling a group preserves each item's individual state.
See [Groups](/scripting/groups/) for patterns.

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
| `ctx.args` | Argument string (exact aliases) |
| `ctx.matches` | Capture array (regex matches) |
| `ctx:remove()` | Remove this item from inside its own callback |

`ctx:remove()` is how a timer stops itself:

```lua
rune.timer.every(10, function(ctx)
    if done then ctx:remove() end
end)
```

## Quarantine

:::caution
A callback that throws **three times in a row** is quarantined: rune
disables that one item and prints a notice, so a broken script can't
flood your screen or wedge the input pipeline. Everything else keeps
running.
:::

To recover, fix the error and re-enable the item — `h:enable()`,
`rune.trigger.enable("name")`, or just `/reload` (re-registering
resets the failure count). One successful run also clears the count.

## Source attribution

Every registration records the registering script's `file:line`. It
shows up in error messages and in the listing commands — `/aliases`,
`/triggers`, `/timers`, `/hooks`, `/binds`, `/bars` — so you can always
tell which script owns an item.

## Managing without handles

Every registry exposes the same management functions, taking the item
name: `enable`, `disable`, `remove`, plus `list()`, `count()`,
`clear()`, and `remove_group(group)`. The full contract is in the
[API reference](/reference/api/#managing).

**Related:** [API reference overview](/reference/api/) ·
[Triggers](/scripting/triggers/) · [Aliases](/scripting/aliases/) ·
[Groups](/scripting/groups/)
