---
title: Bars
description: Script-rendered status displays placed through named bar leaves in a layout tree.
---

A bar is a render function registered under a name. Place it with a `bar` leaf
whose `name` is that registry name:

```lua
rune.ui.bar("clock", function(width)
    return os.date("%H:%M")
end)

rune.ui.layout({
    type = "column",
    children = {
        { type = "pane", name = "output", border = "none" },
        { type = "bar", name = "clock" },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})
```

Rune calls active renderers every 250ms, or immediately after
`rune.ui.refresh_bars()`. A bar displays only when the tree contains a leaf
with `type = "bar"` and its registered `name`, the bar is enabled, its renderer
produces visible content, and every ancestor region is visible. A same-named
pane is a separate resource and never substitutes for the bar.

## Callback width and assigned width

The `width` passed to a bar callback is always the full terminal width, even
when the layout gives that bar a narrower slot. Rune clips the returned content
to the leaf's assigned width:

```lua
rune.ui.bar("vitals", function(terminal_width)
    return "HP 312/340"
end)

-- Normal input prefers three rows, so this auto-height row is three rows.
-- The callback receives the terminal width. The one-row bar content
-- occupies one quarter of the row's width and is clipped to that rectangle.
{
    type = "row",
    size = "auto",
    children = {
        { type = "input", size = "3fr" },
        { type = "bar", name = "vitals", size = "1fr" },
    },
}
```

Use the callback width for terminal-wide policy or formatting, but keep any bar
that may be placed in a narrow slot compact.

## Render results

Return a string, or a table with any of `left`, `center`, and `right`:

```lua
return { left = "HP 312/340", right = "LIVE" }
```

Style freely with `rune.style`. Bars render one row and default to automatic
size in a column, so they take one row when they contain text. Use an explicit
width when placing one in a row. Returning `""` or `nil`, disabling the bar, or
removing it makes its leaf take no space; its siblings reclaim the space.

## Vitals example

A vitals bar fed by GMCP, with a full walkthrough in the
[HP bar cookbook](/cookbook/hp-bar/):

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

Keep renderers cheap. Store or precompute data in the event that changes it,
then format the current snapshot in the bar callback.

## The status bar

The default status bar is registered by the core scripts under the name
`status`. Its leaf is
`{ type = "bar", name = "status" }`. Registering your own
renderer under `status` replaces it completely. The default renderer also shows
tab-completion matches and the Ctrl+C quit warning, so a replacement takes
those over too.

## Managing bars

Use `rune.bars.disable`, `enable`, `toggle`, and `remove` with the registered name.
`toggle` returns `true` when the bar exists and `false` for an unknown name. The
full list is in the [API reference](/reference/api/#managing). `/bars` lists
every bar with its state, group, and registration location.

A renderer that errors three times in a row is
[quarantined](/scripting/model/#quarantine). Re-registering its name gives it
a fresh start.

Bar registration and enabled state survive layout replacement. `/reload`
rebuilds the Lua registry, so scripts register bars again with fresh enabled
and failure state. Region visibility is separate placement state; replacing or
reloading a layout resets regions to their declared `hidden` values.

**Related:** [rune.ui reference](/reference/api/ui/),
[Layout & UI](/interface/layout/),
[Panes](/interface/panes/)
