---
title: Panes
description: Named output buffers you can dock in the layout, write to from triggers, and toggle from binds.
---

A pane is a named output buffer with its own scrollback, docked beside the main
output. Nothing routes text into one automatically: a pane shows the lines your
scripts write to it, which makes it the place to put a category of output you
want kept separate, such as chat, tells, combat, or auction spam.

```lua
rune.pane.create("chat")                    -- optional; write auto-creates
rune.pane.write("chat", styled_text)
rune.pane.toggle("chat")                    -- flip visibility (panes start hidden)
rune.pane.show("chat")                      -- or set it outright
rune.pane.hide("chat")
rune.pane.clear("chat")
```

Dock a pane via the layout. Since `rune.ui.layout` replaces the whole
layout, keep the bottom dock in it:

```lua
rune.ui.layout({
    top    = { { name = "chat", height = 10 } },
    bottom = { "input", "status" },
})
```

A pane's configured `height` is its standalone height. Omit `height` or set it
to `0` to use the default height of 12 lines; any other value below 2 is
treated as 2. The titled header and bottom border use two rows, and the rest
show content. When visible panes are adjacent in the same dock, Rune omits the
upper pane's bottom border and uses the lower pane's titled header as their
shared boundary. Each pane still shows the same number of content rows, so
each shared boundary saves one row. For example, two adjacent panes with
`height = 12` each still show 10 content rows apiece but occupy 23 rows
together instead of 24.

Panes start hidden; `toggle` shows them. A hidden pane keeps accumulating
writes (the buffer is capped at 1000 lines), so toggling it back shows the
recent history. Lines longer than the pane width soft-wrap, and re-fit when
the terminal resizes.

## Scrolling

Every pane scrolls its own buffer; the special name `"main"` is the
output viewport (that's what the default PageUp/PageDown/Ctrl+Home/Ctrl+End
binds target). Aim a pane with binds of your own:

```lua
rune.bind("shift+pgup",   function() rune.pane.scroll_up("chat", 5) end)
rune.bind("shift+pgdown", function() rune.pane.scroll_down("chat", 5) end)
```

While scrolled, the pane freezes on the history you're reading and its
header shows `chat · scroll +N` as new lines land; `scroll_down` past
the end (or `scroll_to_bottom`) returns it to live tailing.

## The mirror pattern

Panes work well when triggers copy (or move) categories of lines into
them:

```lua
-- Copy: line stays in the main window AND lands in the pane
rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
    rune.pane.write("chat", ctx.line:raw())
end)

-- Move: gag it from the main window, keep it in the pane
rune.trigger.regex("^\\[Auction\\]", function(m, ctx)
    rune.pane.write("auctions", ctx.line:raw())
    return false
end)
```

Bind a key to peek, as in the
[quake console](/cookbook/quake-console/) recipe:

```lua
rune.bind("`", function() rune.pane.toggle("chat") end)
```

**Related:** [rune.pane reference](/reference/api/pane/),
[Layout & UI](/interface/layout/),
[Triggers](/scripting/triggers/)
