---
title: Panes
description: Named output buffers you can place in a layout tree, write to from triggers, and toggle from binds.
---

A pane is a named output buffer with its own scrollback. Rune routes server text
to the reserved `output` pane; every other pane shows only lines your scripts
write to it. That makes panes useful for chat, tells, combat, maps, or auction
traffic that should stay separate from normal output.

```lua
rune.pane.write("chat", styled_text)        -- creates the buffer if needed
rune.pane.replace("vitals", block)          -- redraw a whole pane in one update
rune.pane.toggle("chat")                    -- flip the pane's layout placement
rune.pane.show("chat")
rune.pane.hide("chat")
rune.pane.is_hidden("chat")                -- nil when the layout has no chat pane
rune.pane.clear("chat")
```

Place the buffer with a pane leaf:

```lua
rune.ui.layout({
    type = "column",
    children = {
        { type = "pane", name = "chat", size = 10 },
        { type = "pane", name = "output", border = "none" },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})
```

`name = "chat"` binds the leaf to the buffer used by
`rune.pane.write("chat", ...)`. The `size = 10` is a height because the pane
is a child of a column. Put the same pane in a row and its size is a width.

Placing a pane shows it. The buffer is created on first placement or first
`write`, whichever comes first, so a declared pane is visible immediately and
an empty one renders as an empty titled box. Declare a pane that starts hidden
with `hidden = true` on its leaf; `show`, `hide`, and `toggle` change that
local hidden state at runtime and return `true` only when the layout places the
pane. Hiding a pane removes its leaf from the resolved layout but does not
clear its buffer or scroll position, so showing it later reveals recent
history. Buffers survive layout replacement and `/reload`; visibility returns
to the declared `hidden` values.

To preserve visibility while rebuilding a layout, use
`hidden = rune.pane.is_hidden("group") or false` in the new pane declaration.

## Status panes

For vitals, a roster, or another state display, build one string and call
`replace`. It clears and writes in a single UI update, avoiding an empty
frame between separate calls.

```lua
rune.gmcp.on("group", function(group)
    local lines = { "Group: " .. group.groupname .. "  Leader: " .. group.leader }
    for _, member in ipairs(group.members) do
        lines[#lines + 1] = string.format("%-12s %6d/%-6d", member.name, member.info.hp, member.info.mhp)
    end
    rune.pane.replace("group", table.concat(lines, "\n"))
end)
```

## Borders and titles

A pane is bordered by default. Its assigned rectangle includes a titled top
rule, closing rule, and vertical sides. Adjacent bordered panes with no gap
share their boundary, including corners and junctions.

Configure pane presentation directly on its leaf:

```lua
{
    type = "pane",
    name = "map",
    size = 32,
    title = "Wilderness",
    border = "full",
}
```

A `title` replaces the entire generated header, including the pane name and
automatic scroll-state suffix. Set it to `""` to suppress title text, or omit it
when the generated suffix should remain visible. `border = "full"`, the
default, draws all four sides. `border = "none"`
gives the whole assigned rectangle to content. `border = "horizontal"` draws
only the titled top and closing bottom rules. Lines in user-created panes
soft-wrap at render time and re-fit when the terminal or layout changes. The
reserved output transcript retains its append-time wrapping.

See [Layout & UI](/interface/layout/) for nested sidebars and size constraints.

## Scrolling

Every pane scrolls its own buffer. The default PageUp, PageDown, Ctrl+Home, and
Ctrl+End bindings target the reserved `output` pane.

```lua
rune.pane.scroll_up("output", 20)
rune.pane.scroll_up("chat", 5)

rune.bind("shift+pgup", function()
    rune.pane.scroll_up("chat", 5)
end)
rune.bind("shift+pgdown", function()
    rune.pane.scroll_down("chat", 5)
end)
```

While scrolled, a pane stays anchored on the history you are reading. New
writes continue landing in the buffer. With the default generated title, its
header shows `chat · scroll +N`; a custom `title` replaces that suffix
along with the pane name. `scroll_down` past the end or `scroll_to_bottom`
returns the pane to live tailing. User-created panes scroll by logical lines as
written; the specialized output transcript scrolls its stored physical rows.

## The mirror pattern

Triggers commonly copy or move categories of output into panes:

```lua
-- Copy: keep the line in output and also write it to chat.
rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m, ctx)
    rune.pane.write("chat", ctx.line:raw())
end)

-- Move: gag the line from output after writing it to auctions.
rune.trigger.regex("^\\[Auction\\]", function(m, ctx)
    rune.pane.write("auctions", ctx.line:raw())
    return false
end)
```

Bind a key to reveal a pane only when needed:

```lua
rune.bind("`", function()
    rune.pane.toggle("chat")
end)
```

A pane displays only when all conditions are true: the active layout contains
its pane leaf, that placement is not hidden, and every ancestor region is
visible. Creating or writing a buffer does not add a placement to the tree.
Hiding a region does not change any descendant pane placement's hidden state.

**Related:** [rune.pane reference](/reference/api/pane/),
[Layout & UI](/interface/layout/),
[Triggers](/scripting/triggers/)
