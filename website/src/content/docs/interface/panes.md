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
rune.pane.is_visible("chat")                -- nil when the layout has no chat pane
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
placement gate at runtime and return `true` only when the layout places the
pane. Hiding a pane removes its leaf from the resolved layout but does not
clear its buffer or scroll position, so showing it later reveals recent
history. Buffers survive layout replacement and `/reload`; visibility returns
to the declared `hidden` values.

Scripts that rebuild the layout at runtime, for example to swap a set of top
panes, therefore see every pane return to its declared state. Declare a pane
that should normally stay out of the way with `hidden = true`, and when the
current visibility should carry across the rebuild, snapshot it first and
restore both directions afterwards:

```lua
local was_visible = rune.pane.is_visible("group")
rune.ui.layout(build_layout(tab))
if was_visible == true then
    rune.pane.show("group")
elseif was_visible == false then
    rune.pane.hide("group")
end
```

Rune publishes the layout once per script entry, after the callback returns,
so the screen never shows the state between the rebuild and the restore.

## Status panes

A pane that shows current state rather than a log, such as vitals, a group
roster, or an enemy list, is redrawn as a whole block. Build the block as one
string and hand it to `rune.pane.replace`, which empties and refills the pane
in one update. Clearing and then writing line by line sends each step to the
terminal separately, so a frame can land while the pane is empty and the pane
appears to flicker. With `replace` no debounce is needed; redraw as often as
the data changes.

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

During normal allocation, a four-sided pane has an intrinsic minimum of two
columns and two rows, while a horizontal-only pane has a two-row minimum. A
borderless pane adds no chrome minimum. On a terminal too small to satisfy all
minima, Rune may clip below those sizes rather than draw outside the screen.

## Side-by-side panes

Rows and columns can nest around panes and bars:

```lua
rune.ui.layout({
    type = "column",
    children = {
        {
            type = "row",
            children = {
                { type = "pane", name = "output", size = "3fr", border = "none" },
                {
                    type = "column",
                    size = "1fr",
                    min_size = 24,
                    children = {
                        { type = "pane", name = "chat", size = "2fr" },
                        { type = "pane", name = "map", size = "1fr" },
                    },
                },
            },
        },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})
```

The output pane receives three shares of the width and the sidebar one. Inside
the sidebar, chat receives twice the remaining height of the map. A hidden
chat or map pane is pruned before allocation, so the other pane reclaims the
available sidebar space.

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
Hiding a region does not change any descendant pane placement's own gate.

**Related:** [rune.pane reference](/reference/api/pane/),
[Layout & UI](/interface/layout/),
[Triggers](/scripting/triggers/)
