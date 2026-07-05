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
auto-trims to the newest 500 when exceeded. Lines longer than the pane
width soft-wrap at render time, so they re-fit on resize.

## Scrolling

The `scroll_*` functions work on any pane by name. The special name
`"main"` is the output viewport — that's what the default
PageUp/PageDown/Home/End binds target:

```lua
rune.pane.scroll_up("main", 20)     -- the output viewport
rune.pane.scroll_up("chat", 5)      -- a named pane's own buffer
```

A scrolled pane freezes on the history you're reading: new writes keep
landing in the buffer and the pane's header shows
`name · scroll +N` until you return with `scroll_down` or
`scroll_to_bottom`. Hiding a pane (`toggle`) also returns it to the
live tail. Scrolling counts logical lines (as written), not wrapped
rows.

Aim scrolling with binds:

```lua
rune.bind("shift+pageup",   function() rune.pane.scroll_up("chat", 5) end)
rune.bind("shift+pagedown", function() rune.pane.scroll_down("chat", 5) end)
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
