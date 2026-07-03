---
title: Bars
description: Single-line, script-rendered status displays. The built-in status bar is one you can replace.
---

A bar is a render function. Rune calls it with the available width every
250ms (or immediately on `rune.ui.refresh_bars()`) and docks the result:

```lua
rune.ui.bar("clock", function(width)
    return os.date("%H:%M")
end)

rune.ui.layout({ bottom = { "input", "clock", "status" } })
```

A bar displays only if its name appears in the
[layout](/interface/layout/).

## Render results

Return a string, or a table with any of `left`, `center`, `right`:

```lua
return { left = "HP 312/340", right = "LIVE" }
```

Style freely with `rune.style`; bars are plain text.

## Examples

A vitals bar fed by GMCP (full walkthrough in the
[cookbook](/cookbook/hp-bar/)):

```lua
local vitals = {}
rune.gmcp.subscribe("Char")
rune.gmcp.on("Char.Vitals", function(data)
    vitals = data
    rune.ui.refresh_bars()
end)

rune.ui.bar("vitals", function(width)
    if not vitals.hp then return "" end
    return string.format("HP %s/%s  SP %s/%s",
        vitals.hp, vitals.maxhp, vitals.sp, vitals.maxsp)
end)
```

A bar that only appears when it has something to say. Return `""` and the
bar takes no space:

```lua
rune.ui.bar("afk", function(width)
    if not rune.group.is_enabled("afk") then return "" end
    return rune.style.yellow("AFK: tells forwarding to Telegram")
end)
```

## The status bar

The default status bar (connection dot, address, SCROLL/LIVE indicator) is
registered in the core scripts as the bar named `status`; `/bars` shows it.
Register your own render function under the same name to replace it
entirely. The default renderer also displays tab-completion matches while
you cycle and the Ctrl+C quit warning, so a replacement takes those over
too.

## Managing

By name: `rune.bars.disable/enable/remove(name)`, and `rune.bars.list()`
returns everything registered. In the client, `/bars` lists every bar with
its state, group, and the `file:line` that registered it.

## Gotchas

- Render functions run four times a second. Keep them cheap: precompute in
  the event that changes the data (as the GMCP example does), not in the
  renderer.
- A renderer that errors three times in a row is quarantined like any other
  callback. Re-registering the name gives it a fresh start.

**Related:** [Layout & UI](/interface/layout/),
[Panes](/interface/panes/)
