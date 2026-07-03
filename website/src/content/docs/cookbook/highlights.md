---
title: Highlight & Gag Sets
description: A tidy pattern for maintaining lists of things to color and things to silence.
---

Highlight and gag rules multiply fast. Keep them as data:

```lua
local highlights = {
    { pattern = "tells you",        color = rune.style.cyan },
    { pattern = "You are hit",      color = rune.style.red },
    { pattern = "levels up!",       color = rune.style.green },
    { pattern = "The sun rises",    color = rune.style.yellow },
}

for _, h in ipairs(highlights) do
    rune.trigger.contains(h.pattern, function(m, ctx)
        return h.color(ctx.line:clean())
    end, { group = "highlights" })
end

local gags = {
    "The barkeep polishes a glass",
    "A gentle breeze blows",
    "drops a piece of lint",
}

for _, g in ipairs(gags) do
    rune.trigger.contains(g, nil, { gag = true, group = "gags" })
end
```

## How it works

- A trigger handler that returns a string rewrites the line; here, the
  whole line is re-colored. Later triggers match against the rewritten
  text. See [Triggers](/rune/scripting/triggers/).
- A `nil` action with `gag = true` silences the line with no handler at
  all.
- The [groups](/rune/scripting/groups/) are the off switch: `/group gags
  off` when you suspect you're missing something, `/group highlights off`
  for screenshots.

## Variations

Highlight only a word, not the line, by rewriting with a targeted replace:

```lua
rune.trigger.contains("gold coins", function(m, ctx)
    return (ctx.line:clean():gsub("gold coins",
        rune.style.yellow("gold coins")))
end)
```

Gag but count, to silence spam without losing information:

```lua
local swings = 0
rune.trigger.contains("You swing at", function()
    swings = swings + 1
    rune.ui.refresh_bars()
    return false
end, { group = "gags" })

rune.ui.bar("swings", function() return "swings: " .. swings end)
rune.ui.layout({ bottom = { "swings", "input", "status" } })
```

The layout line is required: a bar renders only if a layout dock names it.
