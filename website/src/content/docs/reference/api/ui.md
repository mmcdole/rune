---
title: rune.ui
description: Full signatures for tree layout configuration, bar renderers, and UI surfaces.
---

Arrange the terminal and register status displays. For task-oriented guides,
see [Layout & UI](/interface/layout/) and [Bars](/interface/bars/).

## Quick reference

```lua
rune.ui.layout(config)               -- replace the complete layout
rune.ui.bar(name, render_fn, opts?)  -- register a bar renderer
rune.bars.get(name)                  -- the bar's handle, or nil
rune.bars.toggle(name)               -- toggle a registered bar
rune.ui.refresh_bars()               -- request an immediate bar render
rune.ui.regions.show(id)             -- show an identified row/column
rune.ui.regions.hide(id)             -- hide an identified row/column
rune.ui.regions.toggle(id)           -- toggle an identified row/column
rune.ui.regions.is_hidden(id)       -- true/false, or nil when unknown
rune.ui.search(opts?)                -- open scrollback search
```

`rune.ui.bar` returns a [handle](/reference/api/#handles) and accepts the
[common option](/reference/api/#options) `group`. A tree layout places the
registered name in `{ type = "bar", name = ... }`. A `name` in `opts` is
ignored with a notice.

## rune.ui.layout

```lua
rune.ui.layout(config)
```

`config` replaces the complete layout. Pass the root node directly:

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

The accepted table is parsed and validated before it replaces the active tree.
Within a Lua generation, each call is atomic: an invalid tree raises
`rune.ui.layout: ...` and leaves that generation's current layout in place.
A successful call returns `true`. A top/bottom table from before the tree
layout is rejected with a printed migration notice and returns `false` without
raising, so the rest of the script still loads; see
[Migrating from top/bottom tables](/interface/layout/#migrating-from-topbottom-tables).
`/reload` starts a fresh generation at the default layout before re-evaluating
`init.lua`; an invalid layout during reload therefore leaves the default active,
not the pre-reload tree. Replacing or reloading a layout resets region and
pane hidden values to the new tree's `hidden` declarations. It does not
clear pane buffers or scroll state. Bar registrations survive layout
replacement but are rebuilt with the Lua VM on reload.

### Node fields

| Field | Applies to | Meaning |
|---|---|---|
| `type` | every node | Required node kind: `row`, `column`, `input`, `pane`, `bar`, or `separator` |
| `name` | `pane`, `bar` | Required non-empty resource name |
| `id` | non-root `row`, `column` | Optional unique region identity; a region containing `input` cannot be hidden |
| `children` | `row`, `column` | Dense array of child nodes; may be empty |
| `size` | non-root nodes | Main-axis cells, percentage, fraction, or automatic height |
| `min_size` | non-root nodes | Minimum cells along the parent's axis |
| `max_size` | non-root nodes | Maximum cells along the parent's axis |
| `gap` | `row`, `column` | Blank cells between active children; default `0` |
| `dividers` | `row`, `column` | Draw a rule between adjacent active children, reusing framed seams when possible; default `false` |
| `hidden` | `pane`, identified non-root `row`, `column` | Initial local hidden state; default `false` |
| `title` | `pane` | Optional string replacing the generated pane title; `""` suppresses title text |
| `border` | `pane` | `"full"`, `"horizontal"`, or `"none"`; default `"full"` |
| `char` | `separator` | One-cell separator character |

Unknown fields are errors. The root cannot set `id`, `hidden`, `size`,
`min_size`, or `max_size`. Leaves cannot set `children`, `gap`, or
`dividers`. `name` is
rejected outside pane and bar leaves. Pane names and bar names are each unique
within the tree. `output` names Rune's reserved, pre-created pane. IDs are
unique, valid only on non-root containers, and identify the regions managed by
`rune.ui.regions`.
A region may contain `input`, but it cannot be declared `hidden` while it does.

Rows allocate widths; columns allocate heights. Containers may nest and may
have zero or one child. Empty containers collapse before allocation.

### Size grammar

| Lua value | Constraint |
|---|---|
| positive integer, such as `40` | fixed cells |
| `"1%"` through `"100%"` | integer percentage of the parent's extent after gaps |
| `"Nfr"`, with positive integer `N` | weighted share of the remaining extent |
| `"auto"` | measured preferred height of the node |
| omitted on `pane`, `row`, or `column` | `"1fr"` |
| omitted on `input`, `separator`, or `bar` | `"auto"` in a column, `"1fr"` in a row |

Numeric strings such as `"40"`, decimal percentages, decimal fractions,
whitespace, and other spellings are rejected. `auto` is valid only when the
node is a child of a column and is the omitted-size default for `input`,
`separator`, and `bar`. Intrinsic width for a child of a row is not supported,
so omitted widths use `1fr`. An auto-height row is measured after its children
receive widths.

`min_size` may be zero. `max_size` is positive, and `min_size` cannot exceed
it. Both use cells along the parent's axis. A fixed size must fall within its
explicit minimum and maximum.

Source limits are explicit: fixed cell counts and `fr` weights are 1 through
16,384; `min_size` is 0 through 16,384; `max_size` is 1 through 16,384; and
`gap` is 0 through 16,384. Percentages remain 1 through 100. A tree may contain
at most 4,096 nodes and may reach depth 64, counting the root as depth 0.

Gaps are removed before percentages and fractions are resolved. Fixed,
automatic, and percentage tracks are reserved before `fr` children divide the
remainder. If the preferred sizes overcommit, Rune shrinks fractions first,
then percentages, then fixed and automatic tracks, without crossing a
satisfiable minimum. Capped or non-growing tracks may leave blank cells at the
end of a container. If minimums cannot fit, gaps and ordinary minimums are
relaxed to keep input reachable on both axes; explicit maxima still apply.

### Leaf types

- `input`: command input, composer, picker, and search. Required exactly once.
- `pane`: a named, scrollable buffer. Requires `name`.
- `bar`: a named Lua bar renderer. Requires `name`.
- `separator`: a one-line rule with optional `char`.

Rune pre-creates the buffer named `output` and routes server output into it.
Place it with `{ type = "pane", name = "output" }`. Its layout placement is
optional but, like any pane name, may occur at most once. Every `rune.pane.*`
operation applies to it. Other pane buffers are created on first placement or
first `write`, or explicitly with `create`. Placing a pane shows it; declare
`hidden = true` on the leaf for a pane that starts hidden.

Input can appear anywhere, including inside an identified region. Its automatic
height follows the active editor/picker/search mode. Pane automatic height
measures content at the assigned inner width, plus frame rows, bounded by the
terminal height and `max_size`.

Ordinary pane buffers store logical lines and re-wrap at their current width.
The `output` pane keeps transcript rows at their append-time width, so existing
server output does not reflow after a resize. While `output` is hidden or
omitted, new rows use its last placement width; before its first placement they
use the terminal width, or 80 columns if no terminal size has arrived. Pane
presentation fields are set directly on the leaf:

```lua
{
    type = "pane",
    name = "chat",
    title = "Chat",
    border = "full",
}
```

- `title` (string): supplies the complete header text and replaces the pane name
  and dynamic scroll-state suffix. An empty string suppresses title text; an
  omitted field uses the generated header for ordinary panes. The reserved
  `output` pane is untitled by default, including while scrolled; an explicit
  `title` still supplies its header.
- `border` (string): `"full"`, the default, draws all four
  sides; `"none"` draws none; `"horizontal"` draws titled top and closing
  bottom rules without side walls.

The assigned pane rectangle includes its border. Adjacent bordered panes with
`gap = 0` share a boundary. Setting `border = "none"` gives all assigned cells
to pane content. During normal allocation, full pane chrome requires at least
two columns and two rows, `"horizontal"` requires at least two rows, and a
borderless pane adds no chrome minimum. A smaller explicit `max_size` is a hard
cap and intentionally clips the chrome. Tiny-terminal fallback may also clip
below intrinsic minima when the screen cannot satisfy every constraint.

Separator fields are also direct:

```lua
{
    type = "separator",
    char = "═",
}
```

Omitting `char` uses `─`. A supplied value must occupy exactly one terminal
cell or layout validation fails.

### Regions and pruning

An `id` identifies a non-root row or column:

```lua
rune.ui.regions.show("sidebar")       -- true if found, false otherwise
rune.ui.regions.hide("sidebar")       -- nil, err if it contains input
rune.ui.regions.toggle("sidebar")     -- nil, err if hiding would remove input
rune.ui.regions.is_hidden("sidebar")  -- local hidden value; nil if unknown
```

`hidden` sets the initial local state. The query does not include ancestors,
resource activity, or available screen space. A region containing input may be
identified but cannot be hidden. [Pane visibility](/reference/api/pane/#placement-and-visibility)
uses the same local state, addressed by name.

Hidden subtrees, hidden panes, empty/disabled bars, and containers with no
active children are omitted before sizing. Remaining siblings reclaim the
space. Replacing or reloading the layout restores declared hidden values.

## rune.ui.bar

```lua
rune.ui.bar(name, render_fn, opts?) -> handle
```

- `name` (string): registry name; place it with
  `{ type = "bar", name = name }` in a tree layout.
- `render_fn` (function): `function(width)`, called roughly every 250ms with
  the full terminal width. Return a string, a `{left, center, right}` table, or
  `nil` to produce no content.
- `opts` (table, optional): [common options](/reference/api/#options).

The callback width remains the terminal width even when the layout assigns the
bar a narrower rectangle. Rune clips the rendered block to that assigned width.
A non-empty bar occupies one row; an empty bar takes no space.

A renderer that errors on three consecutive renders is
[quarantined](/reference/api/#quarantine). Re-registering the name gives it a
fresh start.

```lua
rune.ui.bar("clock", function(width)
    return { right = os.date("%H:%M") }
end)
```

`rune.ui.refresh_bars()` requests an immediate render instead of waiting for
the next tick. Call it after changing state read by a renderer.

## rune.ui.search

```lua
rune.ui.search(opts?)
```

- `opts` (table, optional): `query` (string), the initial search text. Omitted
  or empty reopens the overlay with the previous query.

Search scans the output pane's history, newest first, using a
case-insensitive substring match. The viewport follows the selected match.
`Ctrl+F` and `/find [pattern]` open the same surface.

| Key | Action |
|---|---|
| type or Backspace | edit and rescan |
| Up or Down | move to a newer or older match |
| Enter | close and keep the selected scroll position |
| Esc or Ctrl+C | close and restore the previous scroll position |

## Managing bars

Registry management uses the bar name:
`rune.bars.get/enable/disable/toggle/remove(name)`, `.list()`, `.count()`,
`.clear()`, and `.remove_group(group)`. `toggle` returns `true` when the named
bar exists and `false` otherwise. See
[Registries](/reference/api/#managing). `/bars` lists registered bars.

**Related:** [Bars guide](/interface/bars/),
[rune.pane](/reference/api/pane/),
[State & Lines](/reference/api/state-lines/)
