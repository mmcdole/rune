---
title: Layout & UI
description: The dock model, and how bars, panes, the input line, and the output viewport fit together.
---

Rune's screen is one main output viewport with docks above and below.
Docks hold bars (single-line, script-rendered), panes (multi-line output
buffers), and the built-ins `input` and `separator`. You declare the
arrangement once:

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

## What a dock can hold

**[Bars](/interface/bars/)** are single-line displays rendered by your
script. Bars are pulled: four times a second, rune calls the bar's
render function with the current width and shows what it returns. Call
`rune.ui.refresh_bars()` when a bar should update sooner than the next
tick. A renderer usually reads state your script keeps, such as vitals
stored by a GMCP handler, and can also read
[`rune.state`](/reference/api/state-lines/) for client facts like the
connection status. The default status bar is simply a bar named
`status` that the core scripts register; register your own renderer
under that name to replace it.

**[Panes](/interface/panes/)** are multi-line buffers that show their
most recent lines. Panes are pushed: nothing polls them. A pane shows
the lines your script appends with `rune.pane.write()` as they arrive,
from a trigger, for example.

**`input`** is the built-in [command line](/interface/input/). Its
height is intrinsic, so the layout only decides where it sits.

**`separator`** is a built-in one-line horizontal rule. Its `char`
option sets the character the rule repeats:
`{ name = "separator", char = "═" }` draws a double rule. The character
must fit in a single terminal cell; anything else falls back to the
default `─`. For a rule with color, patterns, or text, write a bar
instead.

## Rules of the layout table

- A table entry (`{ name = ..., height = n }`) sets an explicit height
  in lines. For a pane, omit `height` or set it to `0` to use the default
  height of 12 lines; any other value below 2 is treated as 2. The value
  is the pane's standalone height: two rows frame its content with a
  titled header and bottom border. Adjacent visible panes still show the
  same number of content rows but share a boundary: the upper pane drops
  its bottom border, and the lower pane's titled header acts as the
  divider. Two adjacent panes with `height = 12` therefore occupy 23 rows
  instead of 24 while each still shows 10 content rows. Any other
  string-valued key is an option for the named component, like the
  separator's `char`; unknown keys are ignored.
- `rune.ui.layout` replaces the whole layout. Always include `"input"`
  in a dock, because nothing re-adds it if you leave it out.
- A component appears only if a dock names it. Registering a bar or
  writing to a pane is not enough; the layout decides what shows.
  Unknown names are skipped, and hidden panes and empty bars take no
  space.
- The default layout is `bottom = { "input", "status" }`. You only need
  `rune.ui.layout` to change it.

The [quake console recipe](/cookbook/quake-console/) combines bars,
panes, and layout in one short script.

**Related:** [rune.ui reference](/reference/api/ui/),
[Bars](/interface/bars/),
[Panes](/interface/panes/),
[Pickers](/interface/pickers/)
