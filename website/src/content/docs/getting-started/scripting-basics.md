---
title: Scripting Basics
description: Where config lives, the edit/reload loop, and a map of the rune API.
---

Rune is configured in Lua. There is no separate trigger syntax or settings
file. One script runs at startup, and everything is registered from it:

```
~/.config/rune/init.lua
```

## The edit/reload loop

Edit `init.lua`, type `/reload`, keep playing.

`/reload` throws away the Lua state and runs your scripts again from
scratch. A script error doesn't take the client down: it's reported with
its `file:line`, and the rest of your config loads normally. Fix the line
and `/reload` again.

Two commands are useful while iterating:

- `/lua <code>` runs a one-liner without touching a file:
  `/lua rune.echo(rune.state.address)`
- `/aliases`, `/triggers`, `/timers`, `/hooks`, and `/binds` list what's
  registered, including the `file:line` that registered it

## A first script

```lua
-- ~/.config/rune/init.lua

-- An alias: "hp" sends the heal command
rune.alias.exact("hp", "cast 'heal' self")

-- A trigger: highlight incoming hits by rewriting the line
rune.trigger.contains("You are hit", function(matches, ctx)
    return rune.style.red(ctx.line:clean())
end)

-- A timer: save every minute
rune.timer.every(60, "save", { name = "autosave" })
```

Save, then `/reload`.

## Growing past one file

When `init.lua` gets long, split it up with `require()`. Paths resolve
relative to the requiring script, so files next to `init.lua` load without
any path setup:

```
~/.config/rune/
├── init.lua
├── combat.lua
└── ui.lua
```

```lua
-- init.lua
require("combat")   -- runs combat.lua
require("ui")       -- runs ui.lua
```

A required file is plain Lua that runs top to bottom. Put registrations in
it directly; no module table or `return` is needed:

```lua
-- combat.lua
rune.alias.exact("k", "kill target")
rune.trigger.contains("flees in panic", "kill target")
```

Since `/reload` rebuilds the whole Lua state, edits to required files are
picked up on the next `/reload` just like edits to `init.lua`.

## A map of the API

Everything lives under the `rune` table. Find the task, follow the link:

| To do this | Use | Covered in |
|---|---|---|
| React to server output | `rune.trigger` | [Triggers](/rune/scripting/triggers/) |
| Shorten commands you type | `rune.alias` | [Aliases](/rune/scripting/aliases/) |
| Run something later, or on a schedule | `rune.timer` | [Timers](/rune/scripting/timers/) |
| Intercept input, output, and client events | `rune.hooks` | [Hooks & Events](/rune/scripting/hooks/) |
| Bind keys | `rune.bind` | [Key Bindings](/rune/scripting/keybindings/) |
| Add your own `/commands` | `rune.command` | [Slash Commands](/rune/scripting/commands/) |
| Handle GMCP data | `rune.gmcp` | [GMCP](/rune/scripting/gmcp/) |
| Keep data across reloads or restarts | `rune.session`, `rune.store` | [Storage & Worlds](/rune/scripting/storage/) |
| Toggle sets of things at once | `rune.group` | [Groups](/rune/scripting/groups/) |
| Lay out panes, bars, and pickers | `rune.ui`, `rune.pane` | [Layout & UI](/rune/interface/layout/) |
| Color and style text | `rune.style` | [Triggers](/rune/scripting/triggers/) |

The registration functions all behave the same way: each returns a handle
with `:enable()`, `:disable()`, and `:remove()`, and each accepts the same
options table with `name` (registering the same name again replaces the old
entry), `group`, `priority`, and `once`.

## The client itself is Lua

The API above isn't a plugin surface bolted onto the side; the client's
own behavior is built with it. The input pipeline, local echo, the default
keymap, and the status bar are Lua scripts embedded in the binary,
registered through the same hooks your `init.lua` uses. For example, the
`> ` prefix on echoed commands is a hook handler, so you can replace it:

```lua
rune.hooks.on("echo", function(text)
    return rune.style.cyan("» " .. text)
end, { priority = 50 })  -- runs before the default handler
```

Anything the client does through this API, your scripts can override.
