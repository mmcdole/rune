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

```txt
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
- A table entry can also carry component options — extra string-valued
  keys handed to the component it names (see
  [Built-in components](#built-in-components)). Unknown keys are
  ignored.
- `rune.ui.layout` replaces the whole layout. Always include the bottom
  dock with `"input"`, because nothing re-adds the input line if you leave
  it out.
- Unknown names are skipped; hidden panes and empty bars take no space.
- A bar or pane renders only if a dock names it. Registering a renderer
  or writing to a pane is not enough — the layout decides what appears.
- The default layout is `bottom = { "input", "status" }`. You only need
  `rune.ui.layout` to change it.

## Built-in components

- **`input`** — the [command line](/interface/input/) or multiline
  composer. Its height is intrinsic; the layout only decides where it
  sits. Always include it in the bottom dock.
- **`status`** — the default status bar. Registering a bar named
  `"status"` replaces it with your own renderer.
- **`separator`** — a one-line horizontal rule. Its `char` option sets
  the character the rule repeats: `{ name = "separator", char = "═" }`
  draws a double rule. The character must occupy a single terminal
  cell; anything else falls back to the default `─`. For anything
  fancier than a repeated character — color, patterns, text — write a
  [bar](/interface/bars/) instead.

## The pieces

- **[Bars](/interface/bars/)**: you write a render function, and rune
  calls it with the current width every 250ms. The built-in status bar is
  one of these.
- **[Panes](/interface/panes/)**: named buffers that show their most
  recent lines. Write to them from triggers; toggle them from binds.
- **[Pickers](/interface/pickers/)**: fuzzy overlays for commands,
  worlds, and anything your scripts want to offer.

The [quake console recipe](/cookbook/quake-console/) combines all of
this in one short script.

## Pull and push

Bars are pulled. Four times a second, rune calls each bar's render
function and displays whatever it returns. The function reads state
your script keeps, such as vitals stored by a GMCP handler. When that
state changes and the bar should update before the next tick, call
`rune.ui.refresh_bars()`.

Panes are pushed. Nothing polls a pane; it shows the lines you append
with `rune.pane.write()` as they arrive.

Bars can also read [`rune.state`](/reference/api/state-lines/), a
read-only table of client facts like the connection status and terminal
size. The built-in status bar renders from it.

**Related:** [rune.ui reference](/reference/api/ui/),
[Bars](/interface/bars/),
[Panes](/interface/panes/),
[Pickers](/interface/pickers/)
