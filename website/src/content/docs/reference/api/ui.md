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
rune.ui.search(opts?)                -- open the scrollback-search overlay
```

`rune.ui.bar` returns a [handle](/reference/api/#handles) and accepts
the [common options](/reference/api/#options).

### rune.ui.layout

```lua
rune.ui.layout(config)
```

- `config` (table) — `top` and/or `bottom` arrays of dock entries.
  Each entry is a string (the component name) or a table with options:
  `{name = "tells", height = 8}`. The name is a built-in component
  (below), a bar, or a pane. Beyond `name` and `height`, string-valued
  keys are passed to the named component as component options; a
  component ignores keys it doesn't understand.

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

#### Built-in components

- `"input"` — the command line or multiline composer. Height is
  intrinsic; no options.
- `"status"` — the default status bar, shown until a bar named
  `"status"` replaces it. No options.
- `"separator"` — a one-line horizontal rule. Options:
  - `char` (string) — the character the rule repeats:
    `{name = "separator", char = "═"}`. Must occupy a single terminal
    cell; anything else falls back to the default `─`.

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

### rune.ui.search

```lua
rune.ui.search(opts?)
```

- `opts` (table, optional) — `query` (string): the initial search
  text. Omitted or empty reopens the overlay with the previous
  search's query.

Opens the scrollback-search overlay above the input line. It scans the
main window's history (newest first, case-insensitive substring) and
lists matching lines; the viewport follows the selection, centering
each match with the matched text highlighted.

Bound to `Ctrl+F` by default, and available as `/find [pattern]`.
A bare `/find` (or `Ctrl+F`) reopens with the last query preserved;
typing replaces it.

Keys inside the overlay:

| Key | Action |
|-----|--------|
| type / backspace | edit the query (rescans as you type) |
| `Up` / `Down` | step to a newer / older match (the viewport follows) |
| `Enter` | close, leaving the viewport at the match |
| `Esc` / `Ctrl+C` | close and restore the scroll position from before the search |

The match counter shows `current/total`; a total like `250+` means the
scan stopped at its cap and older matches exist beyond it. After
`Enter`, the usual scroll keys apply — `Ctrl+End` returns to live.

## Managing

Standard registry management applies:
`rune.bars.enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). These address the `name`
option, not the bar name. `/bars` lists everything.

**Related:** [Bars guide](/interface/bars/) ·
[rune.pane](/reference/api/pane/) ·
[State & Lines](/reference/api/state-lines/)
