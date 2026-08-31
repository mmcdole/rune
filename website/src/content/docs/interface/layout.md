---
title: Layout & UI
description: Arrange Rune's output, input, bars, and panes with nested rows and columns.
---

A layout describes the whole terminal as one tree. Every node is either a
container or a leaf:

- A `row` lays its children out from left to right.
- A `column` lays its children out from top to bottom.
- A leaf renders `input`, `separator`, a named `bar`, or a named `pane` buffer.
  Server output is the reserved pane named `output`.

The default is a full-width column containing the server output, input, and
status bar:

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

The output pane defaults to `1fr` and takes the height left after the input and
status bar, which default to their intrinsic `auto` heights. Because the root
is a column, all three children inherit its full width. You only need to set a
layout when you want a different arrangement.

## Splitting the screen

Rows and columns can nest. This layout puts the server output beside a
40-column sidebar, then keeps the status and input full-width below both:

```lua
rune.ui.layout({
    type = "column",
    children = {
        {
            type = "row",
            children = {
                { type = "pane", name = "output", border = "none" },
                {
                    type = "column",
                    id = "sidebar",
                    size = 40,
                    min_size = 24,
                    children = {
                        {
                            type = "pane",
                            name = "chat",
                            size = "2fr",
                            title = "Chat",
                            border = "full",
                        },
                        {
                            type = "pane",
                            name = "map",
                            size = "1fr",
                            title = "Map",
                            border = "full",
                        },
                    },
                },
            },
        },
        { type = "bar", name = "status" },
        { type = "input", max_size = 5 },
    },
})
```

Declaring a pane places it: the buffer is created if it does not exist yet,
and an empty pane renders as an empty titled box until a script writes to it.
No setup calls are needed. Only the two pane leaves draw layout frames in this
example.
Containers partition space; they do not add borders around `output`, bars,
input, or themselves.

The container decides which dimension a child's size controls. Inside a row,
`size`, `min_size`, and `max_size` mean width. Inside a column, they mean
height. The root has no parent axis, so it cannot set any of those fields.

## Sizing

Each non-root node accepts one `size`:

| Value | Meaning |
|---|---|
| `40` | Fixed at 40 cells along the parent's axis |
| `"30%"` | 30 percent of the parent's allocatable extent |
| `"2fr"` | Two weighted shares of the space left after fixed, percentage, and automatic tracks |
| `"auto"` | Use the node's preferred height |
| omitted on `pane`, `row`, or `column` | `"1fr"` |
| omitted on `input`, `separator`, or `bar` | `"auto"` |

Cell counts, percentages, and `fr` weights are positive integers.
Percentages are calculated after the container's gaps are removed. Fixed and
automatic tracks take their preferred sizes, percentage tracks take their
share of the parent, and `fr` tracks divide the remainder. Integer rounding is
deterministic, so allocated tracks never overrun the container.

Use `fr` for ratios that should compose with fixed siblings:

```lua
{
    type = "row",
    children = {
        { type = "pane", name = "output", size = "7fr", border = "none" },
        { type = "pane", name = "map", size = "3fr" },
    },
}
```

That is a 70:30 split of the available width. `"70%"` and `"30%"` produce
the same result when those are the only children, but percentages refer to the
whole parent extent. `fr` refers to whatever remains after fixed children.

`min_size` and `max_size` clamp the node along the same parent axis:

```lua
{
    type = "pane",
    name = "map",
    size = "30%",
    min_size = 24,
    max_size = 50,
}
```

When declared sizes do not fit, Rune preserves minimums whenever the terminal
allows and keeps every child inside the available rectangle. The
[API reference](/reference/api/ui/#size-grammar) describes the exact allocation
order.

If fixed, automatic, or capped tracks do not consume the whole parent and no
uncapped `fr` child remains, the unused tail stays blank. Add an unsized pane
or container, or an explicit `fr` child, when the split should fill its axis.

### Automatic height

`"auto"` is supported for a child of a `column`, where size means height. It
is the default for `input`, `separator`, and `bar`. It is not supported for a
child of a `row`; give those leaves an explicit cell, percentage, or `fr` width
there.

An automatic-height row is valid. Rune first assigns widths to the row's
children, measures each child at its assigned width, and uses the tallest
preferred height. This allows wrapped input, search, and other height-aware
leaves to sit beside each other:

```lua
{
    type = "column",
    children = {
        { type = "pane", name = "output", border = "none" },
        {
            type = "row",
            size = "auto",
            children = {
                { type = "input", size = "3fr" },
                { type = "bar", name = "status", size = "1fr" },
            },
        },
    },
}
```

### Output scrollback and width changes

Server output wraps to the `output` pane's width, and existing output rows do
not reflow after a resize or layout change. Named panes re-wrap their lines at
the current width. The [API reference](/reference/api/ui/#leaf-types) covers
output behavior before its first placement and while it is hidden.

## Gaps and pane borders

Set `gap` on a container to reserve blank cells between its active children:

```lua
{
    type = "row",
    gap = 1,
    children = {
        { type = "pane", name = "output", border = "none" },
        { type = "pane", name = "map", size = 32 },
    },
}
```

The gap is subtracted before child sizes are resolved. With `gap = 0`, adjacent
bordered panes share one boundary instead of drawing two borders. Rune owns
the shared frame and draws the correct corners and junctions for nested rows
and columns.

A pane uses a full border by default. The default output placement opts out
with `border = "none"`. Its assigned rectangle includes any border, and the
remaining cells display content. Pane presentation fields are set directly on
the leaf:

```lua
{
    type = "pane",
    name = "map",
    title = "Wilderness",
    border = "full",
}
```

A `title` replaces the pane's entire generated header, including its name and
the automatic `· scroll +N` suffix. Set it to `""` to suppress title text, or
omit it when the generated scroll-state indicator should remain visible. Pane
border values are:

- `"full"`, the default: all four sides.
- `"none"`: no border; the full rectangle belongs to content.
- `"horizontal"`: titled top and closing bottom rules, with no side walls.

Pane borders count toward the leaf's assigned size. A restrictive `max_size` or
a very small terminal may clip them; the
[API reference](/reference/api/ui/#leaf-types) lists their normal minimums.

## Leaves

### Output and input

Rune pre-creates a visible pane resource named `output`; place it like any other
pane with `{ type = "pane", name = "output" }`. Output placement is optional,
so a focused interface may omit it while the buffer continues accumulating.
Like every pane name, `output` may be placed at most once in a tree layout.
Every `rune.pane.*` operation also applies to it.

Every tree layout must contain exactly one `{ type = "input" }`. `input` is
the command line, multiline composer, and picker/search surface. Its omitted
size defaults to `auto`, so wrapped text and the composer can grow vertically
when it is a child of a column.

### Bars

A registered bar is placed through a `bar` leaf. Its `name` is the bar's
registry name:

```lua
rune.ui.bar("vitals", function(width)
    return "HP 312/340"
end)

-- As a child of a column, the default auto size gives a non-empty bar one row:
{ type = "bar", name = "vitals" }

-- As a child of a row, use a fixed, percentage, or fractional width:
{ type = "bar", name = "vitals", size = "1fr" }
```

A `bar` leaf selects only a bar resource. An absent, disabled, or empty bar
never turns into a same-named pane. A pane and a bar may share a name because
their leaf types select different resource namespaces.

The render callback receives the full terminal width even when the tree gives
the bar a narrower slot. Rune clips the result to that slot. A bar that returns
`""` or `nil`, is disabled, or is removed takes no space, and its siblings
reclaim the room.

### Panes

A pane leaf uses `type = "pane"` and names its buffer:

```lua
{ type = "pane", name = "chat", size = 12 }
```

Use `rune.pane.*` for writes and visibility. See [Panes](/interface/panes/).

### Separator

`separator` is a one-line built-in. Set its character directly on the leaf:

```lua
{
    type = "separator",
    char = "═",
}
```

Omit `char` to use `─`. A supplied character must occupy exactly one terminal
cell or layout validation fails.

## Regions and inactive resources

A non-root `row` or `column` becomes an addressable region when it has an `id`.
Regions are the only nodes that accept `id`; `hidden` is valid on regions and
on pane placements:

```lua
{
    type = "column",
    id = "sidebar",
    hidden = true,
    size = 32,
    children = {
        { type = "pane", name = "chat", size = "2fr" },
        { type = "pane", name = "map", size = "1fr" },
    },
}
```

Use the region API to change that placement gate at runtime:

```lua
rune.ui.regions.show("sidebar")        -- true when the region exists
rune.ui.regions.hide("sidebar")        -- true when the region exists
rune.ui.regions.toggle("sidebar")      -- true when the region exists
rune.ui.regions.is_visible("sidebar")  -- true/false, or nil when unknown
```

`is_visible` reports that region's own gate. It does not include a hidden
ancestor or the visibility/activity of panes and bars below it. Visibility
state belongs to the installed layout: replacing the layout or running
`/reload` restores the new declaration's `hidden` values. A region cannot
contain `input`, directly or through nested containers. The output pane may be
placed in a region; its own placement gate and the ancestor region gate both
have to be on for it to render.

Pane placements carry the same gate, addressed by name instead of id:
`hidden = true` on a pane node declares it hidden, and `rune.pane.show`,
`hide`, and `toggle` change it at runtime. See
[Panes](/interface/panes/) for the pane-side view of this.

Rune removes inactive nodes before allocating space:

- A hidden region removes that container and all descendants.
- A hidden pane placement is removed.
- An inactive bar or a bar with no visible content is removed.
- A container with no active descendants is removed.

The remaining siblings reclaim the space. A container that is left with one
active child continues resolving that child normally. Pane buffers and scroll
position survive layout replacement and `/reload`.

## Legacy top/bottom tables

Existing configurations may use a `top`/`bottom` table, with an optional
`version = 1`:

```lua
rune.ui.layout({
    version = 1,
    top = { { name = "chat", height = 10 } },
    bottom = { "input", "status" },
})
```

Rune places top entries first, inserts the reserved `output` pane, and then
places bottom entries. A non-empty legacy table replaces the complete default
and may omit input, so include `"input"` deliberately. Use the tree form for
nested splits, regions, typed pane presentation, and flexible sizing. See the
[API reference](/reference/api/ui/#version-1-tables) for the exact legacy entry
and name-resolution rules.

## Validation

Rune validates the complete tree before replacing the active layout. If
validation fails, the active layout is unchanged. The key rules are:

- The table passed to `rune.ui.layout` is the root node itself.
- Every node has a `type`; pane and bar names are unique within their own
  resource type.
- Rows and columns have a dense `children` array with at least two declared
  children.
- `input` occurs exactly once. Output is the optional reserved pane named
  `output`.
- Region IDs belong only to non-root rows and columns, and a region cannot
  contain input.
- Unknown fields and invalid or contradictory sizing rules are errors.

The [API reference](/reference/api/ui/#runeuilayout) lists every field, limit,
and validation rule, including reload behavior.

**Related:** [rune.ui reference](/reference/api/ui/),
[Bars](/interface/bars/),
[Panes](/interface/panes/),
[Pickers](/interface/pickers/)
