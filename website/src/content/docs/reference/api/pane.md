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
rune.pane.replace(name, text)          -- empty the buffer and write text in one update
rune.pane.show(name)                   -- show the pane's placement in the layout
rune.pane.hide(name)                   -- hide the pane's placement in the layout
rune.pane.toggle(name)                 -- flip the placement's visibility
rune.pane.is_hidden(name)             -- local hidden value; nil when not placed
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

## Replacing contents

`replace` empties the buffer and writes `text` as one UI update. Use it for
panes that are redrawn as a block, such as a status form or a group roster:
`clear` followed by `write` calls sends each step to the terminal separately,
and a frame can land between them and show the pane empty. `replace` creates
a missing buffer like `write` does, and a scrolled pane returns to live
tailing so the new content is what shows. On the `output` pane it also drops an
active search and resets the viewport, exactly like `clear`, and keeps the
live prompt.

```lua
rune.gmcp.on("char.vitals", function(v)
    rune.pane.replace("vitals", table.concat({
        "HP " .. v.hp .. "/" .. v.maxhp,
        "MP " .. v.mp .. "/" .. v.maxmp,
    }, "\n"))
end)
```

## Placement and visibility

Visibility belongs to the placement, not the buffer. `show`, `hide`, and
`toggle` flip the pane's placement in the active layout tree and return `true`
when the layout places the pane, `false` otherwise. `is_hidden` reports the
placement's local hidden state, or `nil` when the layout does not place the pane.
Declare `hidden = true` on a pane node to start it hidden; see the
[layout guide](/interface/layout/#visibility-and-regions).

The pane displays only while its own placement is visible and every ancestor
region is visible. A hidden placement is pruned before space is allocated.
Hiding a region does not alter the pane placement's hidden state. Buffer contents
and scroll position survive layout replacement and `/reload`; runtime
visibility changes last until the next `rune.ui.layout` or `/reload`, which
restore the declared `hidden` values.

Borders and titles belong to the layout declaration; see
[node fields](/reference/api/ui/#node-fields).

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
