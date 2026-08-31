---
title: rune.pane
description: Full signatures for named scrollback panes, including writing, visibility, clearing, and scrolling.
---

Panes are named, scrollable text buffers. A tree layout places one with
`{ type = "pane", name = name }`. For a task-oriented introduction, see
[Panes](/interface/panes/).

## Quick reference

```lua
rune.pane.create(name)                 -- pre-create the named buffer
rune.pane.write(name, text)            -- append text to the named buffer
rune.pane.show(name)                   -- show the pane's placement in the layout
rune.pane.hide(name)                   -- hide the pane's placement in the layout
rune.pane.toggle(name)                 -- flip the placement's visibility
rune.pane.is_visible(name)             -- placement gate; nil when not placed
rune.pane.clear(name)                  -- empty the buffer; unknown is a no-op
rune.pane.scroll_up(name, lines?)      -- scroll back; default 1
rune.pane.scroll_down(name, lines?)    -- scroll forward; default 1
rune.pane.scroll_to_top(name)          -- jump to the oldest line
rune.pane.scroll_to_bottom(name)       -- return to live tailing
```

Panes are push-based: scripts write lines as events happen. This is the
opposite of [bars](/reference/api/ui/), whose callbacks Rune polls. An ordinary
pane grows to 1000 logical lines, then trims to the newest 500. Its lines
soft-wrap to the assigned content width during rendering, so they re-fit after
a resize or layout change. The specialized `output` pane keeps up to 100,000
physical transcript rows wrapped at append time; existing output therefore does
not reflow after a resize.

A pane's buffer and its placement are separate things. Placing
`{ type = "pane", name = name }` in a layout creates the buffer if needed and
shows it; an empty declared pane renders as an empty titled box. `write`
auto-creates a missing buffer, so content written before the layout exists is
kept. `create` pre-creates a buffer explicitly; `clear` is a no-op for an
unknown name. Rune pre-creates the reserved `output` buffer.

## Placement and visibility

Visibility belongs to the placement, not the buffer. `show`, `hide`, and
`toggle` flip the pane's placement in the active layout tree and return `true`
when the layout places the pane, `false` otherwise. `is_visible` reports the
placement's own gate, or `nil` when the layout does not place the pane.
Declare a pane that starts hidden with `hidden = true` on its node:

```lua
rune.ui.layout({
    type = "column",
    children = {
        {
            type = "pane",
            name = "combat",
            size = 8,
            title = "Combat",
            border = "full",
            hidden = true,
        },
        { type = "pane", name = "output", border = "none" },
        { type = "input" },
    },
})

rune.trigger.regex("^You hit (.+) for (\\d+)", function(m)
    rune.pane.write("combat", "Hit " .. m[1] .. " for " .. m[2])
end)

rune.bind("f5", function()
    rune.pane.toggle("combat")
end)
```

The pane displays only while its own placement is visible and every ancestor
region is visible. A hidden placement is pruned before space is allocated.
Hiding a region does not alter the pane placement's own gate. Buffer contents
and scroll position survive layout replacement and `/reload`; runtime
visibility changes last until the next `rune.ui.layout` or `/reload`, which
restore the declared `hidden` values.

The pane's assigned rectangle includes its border. A `title` replaces the
entire generated header, including the pane name and automatic scroll-state
suffix. Set it to `""` to suppress title text, or omit it for the generated
header. `border = "full"`, the default, draws all four
sides; `"none"` removes pane chrome; `"horizontal"` draws titled top and closing
bottom rules only. Adjacent bordered panes with no container gap share their
boundary.

Pane chrome supplies an intrinsic minimum during normal allocation: two
columns and two rows for a full border, two rows for `"horizontal"`, and no
extra minimum for a borderless pane. If terminal geometry cannot satisfy all
minima, the tiny-terminal fallback may clip below those sizes.

## Scrolling

The `scroll_*` functions operate on a pane by name. Use the reserved `output`
name for server-output scrollback:

```lua
rune.pane.scroll_up("output", 20)
rune.pane.scroll_up("chat", 5)
```

When supplied, `lines` must be a positive integer.

While scrolled, new writes still enter the buffer. The pane remains anchored
on the history being read. With the default title its header shows
`name · scroll +N`; a custom `title` replaces that suffix too. Calling
`scroll_down` until it reaches the end, or calling `scroll_to_bottom`, returns
it to live mode. User-created panes scroll by logical lines as written. The
specialized `output` transcript instead scrolls its stored physical rows.

```lua
rune.bind("shift+pgup", function()
    rune.pane.scroll_up("chat", 5)
end)
rune.bind("shift+pgdown", function()
    rune.pane.scroll_down("chat", 5)
end)
```

**Related:** [Panes guide](/interface/panes/),
[rune.ui](/reference/api/ui/),
[rune.trigger](/reference/api/trigger/)
