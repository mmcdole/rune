---
title: rune.pane
description: Full signatures for scrollable text panes — writing, toggling, clearing, and scrolling.
---

Panes are named, scrollable text buffers you dock in the layout —
chat windows, combat logs, debug output. For a task-oriented
introduction, see [Panes](/interface/panes/).

## Quick reference

```lua
rune.pane.create(name)                 -- create a pane (optional; writes auto-create)
rune.pane.write(name, text)            -- append a line
rune.pane.toggle(name)                 -- show/hide the pane
rune.pane.clear(name)                  -- empty the buffer
rune.pane.scroll_up(name, lines?)      -- scroll back (default 1 line)
rune.pane.scroll_down(name, lines?)    -- scroll forward (default 1 line)
rune.pane.scroll_to_top(name)          -- jump to the oldest line
rune.pane.scroll_to_bottom(name)       -- jump back to live
```

Panes are push-based: you write lines as events happen, and the pane
displays them — the opposite of [bars](/reference/api/ui/), which
pull content from a render function. The buffer holds 1000 lines and
auto-trims to the newest 500 when exceeded.

Docked panes always show their newest lines; per-pane scrolling is not
implemented. The `scroll_*` functions act only on the main output
viewport, under the name `"main"` — that's how the default
PageUp/PageDown/Home/End binds work:

```lua
rune.pane.scroll_up("main", 20)
rune.pane.scroll_to_bottom("main")
```

:::note
A pane displays only when the layout names it — see
[Layout & UI](/interface/layout/). `toggle` shows and hides a pane
that has a dock slot.
:::

The usual shape is a trigger that writes plus a bind that toggles:

```lua
rune.ui.layout({
    top = { {name = "combat", height = 8} },
    bottom = { "input", "status" }
})

rune.trigger.regex("^You hit (.+) for (\\d+)", function(m)
    rune.pane.write("combat", "Hit " .. m[1] .. " for " .. m[2])
end)

rune.bind("f5", function()
    rune.pane.toggle("combat")
end)
```

**Related:** [Panes guide](/interface/panes/) ·
[rune.ui](/reference/api/ui/) ·
[rune.trigger](/reference/api/trigger/)
