---
title: Layout & UI
description: Arrange Rune's output, input, bars, and panes with nested rows and columns.
---

A layout describes the terminal as a tree. A `row` places children left to
right; a `column` places them top to bottom. Leaves display a named pane,
a registered bar, input, or a separator.

Lua core installs this default:

```lua
rune.ui.layout({
    type = "column",
    children = {
        { type = "pane", name = "output", border = "none" },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})
```

The output takes the height left after input and status. You only need to
declare a layout when you want a different arrangement. Every layout must
contain exactly one input; output is optional.

## Splitting the screen

Nest containers to keep input beneath output while a sidebar spans both:

```lua
rune.ui.layout({
    type = "column",
    children = {
        {
            type = "row",
            dividers = true,
            children = {
                {
                    type = "column",
                    children = {
                        { type = "pane", name = "output", border = "none" },
                        { type = "input" },
                    },
                },
                {
                    type = "column",
                    id = "sidebar",
                    size = 32,
                    children = {
                        { type = "pane", name = "chat", size = "2fr", border = "horizontal" },
                        { type = "pane", name = "map", border = "horizontal" },
                    },
                },
            },
        },
        { type = "bar", name = "status" },
    },
})
```

Declaring a pane creates its buffer if needed. An empty pane displays its
frame; scripts fill it with `rune.pane.write` or `rune.pane.replace`.
Containers may have zero, one, or many children. Empty containers take no space.

## Sizing

A child's size follows its parent's axis: width in a row, height in a column.
The root fills the terminal and has no size constraints.

| `size` | Meaning |
|---|---|
| `40` | 40 cells |
| `"30%"` | 30 percent of the parent's allocatable extent |
| `"2fr"` | Two shares of the space remaining after other sizes |
| `"auto"` | Measured height; only valid in a column |
| omitted | `"1fr"`, except input, bars, and separators in a column use `"auto"` |

Use `fr` for ratios that compose with fixed siblings. For example, `3fr`
and `1fr` divide the remaining space 75:25. Percentages refer to the whole
allocatable extent, not the remainder.

`min_size` and `max_size` bound the same dimension:

```lua
{ type = "pane", name = "map", size = "30%", min_size = 24, max_size = 50 }
```

When sizes cannot fit, Rune shrinks tracks within their bounds. If even
minimums cannot fit, it drops gaps and relaxes minimums while protecting input.
An explicit maximum remains a hard cap. Fixed or capped children may leave
unused space at the end; include an uncapped `fr` child to fill it.

### Automatic height

Input measures its current editing mode, a non-empty bar uses one row, and
a separator uses one row. A pane measures its content at the assigned width,
plus frame rows, bounded by the terminal height and `max_size`. Use a fixed
or fractional height for a log that should not grow as lines arrive.

An auto-height row first assigns its children's widths, then uses their tallest
preferred height. Explicit `auto` widths are unsupported; omitted widths
use `1fr`.

## Borders and dividers

Pane rectangles include their borders. Choose `border = "full"` (default),
`"horizontal"` (top and bottom only), or `"none"`. A `title` replaces the
complete generated header, including its scroll-state suffix; `title = ""`
removes the text. Omit it to retain the generated title for ordinary panes.
The reserved `output` pane has no title by default, including while scrolled;
set an explicit `title` to label it.

Containers do not draw outer borders. `dividers = true` draws between active
children: vertical rules in a row, horizontal rules in a column. With no gap,
adjacent pane frames share a cell; a divider uses that seam or reserves a cell
when no frame provides one.

A positive `gap` reserves that many cells between children. With dividers
enabled, the rule sits near the middle of that gap. Shared pane frames,
dividers, default separators, and input rules join at tees and crosses.

A separator is `{ type = "separator" }`. Set `char = "═"` or another
single-cell character for a standalone rule that does not join the frame grid.

## Visibility and regions

Give a non-root container an `id` to address the whole subtree:

```lua
rune.ui.regions.hide("sidebar")
rune.ui.regions.show("sidebar")
rune.ui.regions.toggle("sidebar")
rune.ui.regions.is_hidden("sidebar") -- local hidden value; nil if absent
```

Pane placements use the same operations by name: `rune.pane.hide("chat")`,
`show`, `toggle`, and `is_hidden`. Declare `hidden = true` on a pane or
identified container to start it hidden.

`is_hidden` reports only that node's state. A hidden ancestor does not change
its descendants' flags. A region may contain input, but cannot be hidden while
it does: runtime hide/toggle returns `nil, err`, and a hidden declaration is
rejected.

Hidden nodes, empty/disabled bars, and containers without active children take
no space. Their siblings reclaim it. Installing another layout or running
`/reload` restores the declaration's hidden values; pane buffers and scroll
positions survive.

## Bars and output

A `bar` leaf names a renderer registered with `rune.ui.bar`. Its callback
receives the terminal width, even in a narrower slot. Returning
`{ left = ..., center = ..., right = ... }` lets Rune align fields within the
assigned slot; strings are clipped there. Empty bars collapse. See [Bars](/interface/bars/).

Ordinary panes re-wrap logical lines at their current width. The reserved
`output` pane retains rows wrapped at append time. While hidden or omitted,
new output uses its last placement width. See [Panes](/interface/panes/).

## Toggleable chat pane

Place a hidden pane above output and bind a key to show or hide it:

```lua
rune.ui.layout({
    type = "column",
    children = {
        { type = "pane", name = "chat", size = 10, border = "horizontal", hidden = true },
        { type = "pane", name = "output", border = "none" },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})

rune.bind("f3", function() rune.pane.toggle("chat") end)
```

Panes are visible unless declared with `hidden = true`.

The [API reference](/reference/api/ui/) covers all fields, validation limits,
and reload behavior.
