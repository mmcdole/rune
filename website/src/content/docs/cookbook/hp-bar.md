---
title: HP Bar from GMCP
description: A block-graph vitals bar that updates the instant the server reports them.
---

On a GMCP server, vitals arrive as data, with no prompt parsing. This
renders them as a block graph docked above the input line:

```lua
local vitals = {}

rune.gmcp.subscribe("Char")
rune.gmcp.on("Char.Vitals", function(data)
    vitals = data
    rune.ui.refresh_bars()   -- render now, don't wait for the tick
end)

local function blocks(cur, max, width, color)
    local filled = max > 0 and math.floor(width * cur / max + 0.5) or 0
    return color(string.rep("█", filled)) ..
        rune.style.gray(string.rep("░", width - filled))
end

rune.ui.bar("vitals", function(width)
    local hp, mhp = tonumber(vitals.hp), tonumber(vitals.maxhp)
    if not (hp and mhp) then return "" end
    local sp, msp = tonumber(vitals.sp) or 0, tonumber(vitals.maxsp) or 1
    local hp_color = hp < mhp * 0.25 and rune.style.red or rune.style.green
    return string.format("HP %s %d/%d   SP %s %d/%d",
        blocks(hp, mhp, 14, hp_color), hp, mhp,
        blocks(sp, msp, 10, rune.style.cyan), sp, msp)
end)

rune.ui.layout({ bottom = { "vitals", "input", "status" } })
```

## How it works

- The [GMCP](/scripting/gmcp/) handler stores the data and calls
  `refresh_bars()`; the render function only formats. Keep that split, since
  renderers run four times a second.
- `subscribe("Char")` asks the server for the whole `Char` package.
- The layout line matters: a [bar](/interface/bars/) shows only if a
  layout dock names it.
- The bar turns red under 25% HP by swapping the style function.

## Variations

- Without GMCP, feed `vitals` from a prompt trigger instead:
  `rune.trigger.regex("^HP:(\\d+)/(\\d+)", ...)`. The bar code doesn't
  change.
- Field names (`sp`/`maxsp` here) vary by game; `/gmcp` and the catch-all
  `"gmcp"` hook show what yours sends.
