---
title: rune.clipboard
description: Copy text to the system clipboard.
---

```lua
rune.clipboard.set(text)               -- copy text to the system clipboard
```

The copy is done with OSC 52, an escape sequence that asks your
terminal emulator to set the clipboard. Because the terminal does the
work, it also works over SSH, with no clipboard tools needed on the
remote machine.

Support depends on your terminal. Modern emulators (kitty, alacritty,
WezTerm, iTerm2, Windows Terminal, foot) handle it; some cap how much
text one copy can carry. Inside tmux, `set-clipboard` must be `on` or
`external` (the default) in the tmux config.

A typical use is collecting a block with a
[multi-line trigger](/reference/api/trigger/#multi-line-triggers) and
copying it from an alias:

```lua
local note = ""

rune.trigger.starts("-- BEGIN NOTE", function(m, ctx)
    local collected = {}
    for _, line in ipairs(ctx.lines) do
        collected[#collected + 1] = line:clean()
    end
    note = table.concat(collected, "\n")
end, { span = { to = "^-- END NOTE", max = 40 } })

rune.alias.exact("copynote", function()
    rune.clipboard.set(note)
end)
```

There is no `rune.clipboard.get`. Most terminals refuse OSC 52 reads,
since a server that could read your clipboard is a security problem.

**Related:** [rune.trigger](/reference/api/trigger/) ·
[rune.alias](/reference/api/alias/)
