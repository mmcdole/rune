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

A typical use is collecting something with triggers and copying it
from an alias:

```lua
local note = {}
local collecting = false

rune.trigger.starts("-- BEGIN NOTE", function()
    collecting, note = true, {}
end)
rune.trigger.starts("-- END NOTE", function()
    collecting = false
end)
rune.trigger.regex(".*", function(m, ctx)
    if collecting then note[#note + 1] = ctx.line:raw() end
end)

rune.alias.exact("copynote", function()
    rune.clipboard.set(table.concat(note, "\n"))
end)
```

There is no `rune.clipboard.get`. Most terminals refuse OSC 52 reads,
since a server that could read your clipboard is a security problem.

**Related:** [rune.trigger](/reference/api/trigger/) ·
[rune.alias](/reference/api/alias/)
