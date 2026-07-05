---
title: Panes
description: Named output buffers you can dock in the layout, write to from triggers, and toggle from binds.
---

```lua
rune.pane.create("chat")                    -- optional; write auto-creates
rune.pane.write("chat", styled_text)
rune.pane.toggle("chat")                    -- show/hide (panes start hidden)
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

A docked pane renders a title header and a bottom border, which use two of
its `height` lines. Panes start hidden; `toggle` shows them. A hidden pane
keeps accumulating writes (the buffer is capped at 1000 lines), so toggling
it back shows the recent history. Lines longer than the pane width
soft-wrap, and re-fit when the terminal resizes.

## Scrolling

Every pane scrolls its own buffer; the special name `"main"` is the
output viewport (that's what the default PageUp/PageDown/Home/End
binds target). Aim a pane with binds of your own:

```lua
rune.bind("shift+pageup",   function() rune.pane.scroll_up("chat", 5) end)
rune.bind("shift+pagedown", function() rune.pane.scroll_down("chat", 5) end)
```

While scrolled, the pane freezes on the history you're reading and its
header shows `chat · scroll +N` as new lines land; `scroll_down` past
the end (or `scroll_to_bottom`, or hiding the pane) returns it to live
tailing.

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
