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
it back shows the recent history.

Panes always show their newest lines; per-pane scrolling is not
implemented. The `rune.pane.scroll_*` functions act on the main output
viewport under the name `"main"`. This is how the default
PageUp/PageDown/Home/End binds work:

```lua
rune.pane.scroll_up("main", 20)
rune.pane.scroll_down("main", 20)
rune.pane.scroll_to_top("main")
rune.pane.scroll_to_bottom("main")
```

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
