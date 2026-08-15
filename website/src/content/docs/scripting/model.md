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

## Names

A registry name identifies one registration. It is what you pass to `get`,
`enable`, `disable` and `remove`, what a re-registration replaces, and what
`/triggers`, `/binds` and the other listings show.

Some creation functions assign this name automatically. For the others, set
`opts.name` when you want to look up, manage or replace the registration by
name.

| Creation function | Name | Because |
|---|---|---|
| `rune.bind` | the key, `"ctrl+g"` | Only one binding can use a key |
| `rune.ui.bar` | the layout name, `"status"` | Only one renderer can use a layout name |
| `rune.command.add` | the command, `"greet"` | Only one handler can use a command name |
| `rune.alias.exact` | the normalized phrase, `"chat off"` | Only one expansion can use a typed phrase |
| `rune.trigger.*` | `opts.name` | Several triggers may match the same text |
| `rune.alias.regex` | `opts.name` | Several regex aliases may use the same pattern |
| `rune.timer.*` | `opts.name` | Several timers may use the same delay or interval |
| `rune.hooks.on` | `opts.name` | Several handlers may listen to the same event |
| `rune.gmcp.on` | `opts.name` | Several handlers may listen to the same package |

When only one registration can use a value, that value is already its name.
A binding is named by its key, a bar by its layout name, a command by its
command name, and an exact alias by its phrase. When several registrations
can share the same value, give each one its own name with `opts.name`.

Either way you manage it the same way:

```lua
rune.trigger.contains("food", "eat bread", { name = "feeder" })
rune.bind("ctrl+g", toggle_map)

rune.trigger.disable("feeder")
rune.binds.disable("ctrl+g")
```

A registration without a name can still be managed through the handle
returned when it was created:

```lua
local h = rune.trigger.contains("food", "eat bread")
h:disable()
```

Registering the same name again replaces the old entry rather than adding a
second one. That is what keeps `/reload` from stacking a duplicate trigger
each time you edit a script, so name anything you expect to re-register.

Passing `opts.name` to one of the four automatically named functions is
ignored, with a notice telling you the name to manage it by. Automatic naming
also lets you reach the core's own binds, bars and commands, which are
registered without options at all.

## Options

Every creation function takes an optional `opts` table as its last argument.
Three fields work everywhere:

| Option | Type | Default | Description |
|---|---|---|---|
| `group` | string | none | Group membership for batch enable/disable/remove. |
| `priority` | number | 50 | Execution order where multiple items can match (regex aliases, triggers, hooks). Lower runs first. |
| `once` | bool | false | Auto-remove after the first match (aliases, triggers). |

`name` sets the name where the registry does not set its own, as above.

Individual registries add their own options. Triggers take `gag` and `raw`,
for example. Each [API reference page](/reference/api/) lists its extras.

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

Every registry exposes the same functions, taking the item name: `get`,
`enable`, `disable`, `remove`, plus `list()`, `count()`, `clear()`, and
`remove_group(group)`. The full contract is in the
[API reference](/reference/api/#managing).

`get` returns the handle, which is how you reach something you did not
register yourself:

```lua
rune.bars.get("status"):disable()
```

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
h:action()   -- the registered action
```

Methods chain, which is handy for registering something already switched off:

```lua
rune.trigger.contains("Weather:", nil, {gag = true}):disable()
```

`h:action()` gives back what you registered, which is how you extend
something instead of replacing it. Calling the result runs it directly,
skipping the enabled and group checks and the failure quarantine:

```lua
local scroll = assert(rune.binds.get("pgup")):action()
rune.bind("pgup", function()
    scroll()
    rune.echo("scrolled")
end)
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
