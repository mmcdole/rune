---
title: Quake-Style Chat Console
description: A top pane for tells and channels, toggled with backtick, so nothing scrolls away in combat spam.
---

Hit `` ` `` and a chat pane opens above the output pane with every
tell and channel message you've received; hit it again and it's gone. Tells
can't scroll away under combat spam, and they're all in one place when you
come back to the keyboard.

```lua
rune.bind("`", function()
    rune.pane.toggle("chat")
end)

local style = rune.style
local function mirror(tag, color, name, msg)
    rune.pane.write("chat", color("[" .. tag .. "]") .. " " ..
        style.bold(name) .. ": " .. msg)
end

rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m)
    mirror("Tell", style.cyan, m[1], m[2])
end, { group = "chat-console" })

rune.trigger.regex("^You tell (\\w+): (.+)$", function(m)
    mirror("Tell", style.cyan, "-> " .. m[1], m[2])
end, { group = "chat-console" })

rune.trigger.regex("^(\\w+) \\[(\\w+)\\]: (.+)$", function(m)
    mirror(m[2], style.yellow, m[1], m[3])
end, { group = "chat-console" })

rune.ui.layout({
    type = "column",
    children = {
        { type = "pane", name = "chat", size = 10, hidden = true },
        { type = "pane", name = "output", border = "none" },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})
```

## How it works

- The [pane](/interface/panes/) accumulates writes whether visible or
  not; `` ` `` only toggles visibility. A bound printable key fires only on
  an empty input line, so typing backtick mid-sentence still works, and a
  hidden pane takes no screen space.
- The [triggers](/scripting/triggers/) mirror matching lines into the
  pane, restyled compactly. They don't gag, so chat still appears inline
  too. Add `return false` in the handlers to move messages instead of
  copying them.
- The [layout](/interface/layout/) puts the pane first in the root column, 10
  lines tall, declared `hidden = true` so the console starts closed. While it
  is hidden, the output pane reclaims that height.

## Variations

- Match your game's channel formats. The third trigger's
  `Name [Channel]: text` pattern is the part most likely to differ.
- `/group chat-console off` silences the mirroring without touching the
  pane or the bind.
- For timestamps, prepend `style.gray(os.date("[%H:%M] "))` in `mirror`.

**Related:** [Panes](/interface/panes/) · [Triggers](/scripting/triggers/) · [Key Bindings](/scripting/keybindings/) · [Layout & UI](/interface/layout/)
