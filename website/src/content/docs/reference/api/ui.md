---
title: rune.ui
description: Full signatures for layout configuration and bar renderers.
---

Arrange the screen and render status bars. For task-oriented
introductions, see [Layout & UI](/interface/layout/) and
[Bars](/interface/bars/).

## Quick reference

```lua
rune.ui.layout(config)               -- set the dock layout
rune.ui.bar(name, render_fn, opts?)  -- register a bar renderer
rune.ui.refresh_bars()               -- request an immediate re-render
```

`rune.ui.bar` returns a [handle](/reference/api/#handles) and accepts
the [common options](/reference/api/#options).

### rune.ui.layout

```lua
rune.ui.layout(config)
```

- `config` (table) — `top` and/or `bottom` arrays of dock entries.
  Each entry is a string (the component name) or a table with options:
  `{name = "tells", height = 8}`. Built-in components: `"input"` (the
  command line), `"status"` (the default status bar), and
  `"separator"` (a horizontal rule); anything else names a bar or pane.

```lua
-- The default layout
rune.ui.layout({
    bottom = { "input", "status" }
})

-- Add a chat pane to the top dock
rune.ui.layout({
    top = { {name = "tells", height = 8} },
    bottom = { "input", "status" }
})
```

:::note
A bar or pane renders only if a layout dock names it — see
[Layout & UI](/interface/layout/).
:::

### rune.ui.bar

```lua
rune.ui.bar(name, render_fn, opts?) -> handle
```

- `name` (string) — the bar's layout name (`"status"` replaces the
  built-in status bar).
- `render_fn` (function) — `function(width)`; called on the render
  tick (roughly every 250ms) with the terminal width. Return a string,
  a `{left, center, right}` table, or `nil` to skip this render.
- `opts` (table, optional) — [common options](/reference/api/#options).

Bars are pull-based: rune asks your renderer for current content
instead of you pushing updates. A renderer that errors on 3
consecutive renders is [quarantined](/reference/api/#quarantine);
re-registering the name gives it a fresh start. For a status bar
built on client state, see [State & Lines](/reference/api/state-lines/).

```lua
rune.ui.bar("clock", function(width)
    return { right = os.date("%H:%M") }
end)
```

`rune.ui.refresh_bars()` requests an immediate re-render instead of
waiting for the tick — call it after changing the state a renderer
reads, e.g. in a GMCP vitals handler.

## Managing

Standard registry management applies:
`rune.bars.enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). These address the `name`
option, not the bar name. `/bars` lists everything.

**Related:** [Bars guide](/interface/bars/) ·
[rune.pane](/reference/api/pane/) ·
[State & Lines](/reference/api/state-lines/)
