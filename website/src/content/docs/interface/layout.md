---
title: Layout & UI
description: The dock model, and how bars, panes, the input line, and the output viewport fit together.
---

Rune's screen is one main output viewport with docks above and below.
Docks hold bars (single-line, script-rendered), panes (multi-line output
buffers), and the built-in components `input`, `status`, and `separator`.
You declare the arrangement once:

```lua
rune.ui.layout({
    top    = { { name = "chat", height = 10 } },  -- a pane, 10 lines tall
    bottom = { "input", "separator", "status" },  -- input, a rule, the status bar
})
```

Reading that: a chat pane docked on top; at the bottom, the input line,
a separator rule, and the status bar below it.

```
┌─────────────────────────────┐
│ chat pane (10 lines)        │  top dock
├─────────────────────────────┤
│                             │
│ main output viewport        │
│                             │
├─────────────────────────────┤
│ > input line                │  bottom dock
│ ─────────────────────────── │
│ status bar                  │
└─────────────────────────────┘
```

## Rules of the layout table

- Entries are component names: a bar name, a pane name, or the built-ins
  `"input"`, `"status"`, `"separator"`. A table entry
  (`{ name = ..., height = n }`) sets an explicit height in lines (a pane
  spends two of those on its header and bottom border).
- `rune.ui.layout` replaces the whole layout. Always include the bottom
  dock with `"input"`, because nothing re-adds the input line if you leave
  it out.
- Unknown names are skipped; hidden panes and empty bars take no space.
- The default layout is `bottom = { "input", "status" }`. You only need
  `rune.ui.layout` to change it.

## The pieces

- **[Bars](/rune/interface/bars/)**: you write a render function, and rune
  calls it with the current width every 250ms. The built-in status bar is
  one of these.
- **[Panes](/rune/interface/panes/)**: named buffers that show their most
  recent lines. Write to them from triggers; toggle them from binds.
- **[Pickers](/rune/interface/pickers/)**: fuzzy overlays for commands,
  worlds, and anything your scripts want to offer.

The [quake console recipe](/rune/cookbook/quake-console/) combines all of
this in one short script.

## Reactive state

Bars usually render from `rune.state`, a read-only table the client keeps
current: `connected`, `address`, `scroll_mode`, `scroll_lines`, `width`,
`height`. When your own state changes and a bar should reflect it now, call
`rune.ui.refresh_bars()`.

**Related:** [Bars](/rune/interface/bars/),
[Panes](/rune/interface/panes/),
[Pickers](/rune/interface/pickers/)
