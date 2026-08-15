---
title: rune.bind
description: Signatures, options, and registry management for key bindings.
---

Key bindings run Lua callbacks on key presses. For a task-oriented
introduction, see [Keybindings](/scripting/keybindings/).

## Quick reference

```lua
rune.bind(key, callback, opts?)   -- bind a key; rebinding replaces (upsert by key)
rune.unbind(key)                  -- remove a binding; true if one existed
rune.binds.get(key)               -- the binding's handle, or nil
```

`rune.bind` returns a [handle](/reference/api/#handles) and accepts the
[common option](/reference/api/#options) `group`. The key is the bind's
[name](/reference/api/#names), so a `name` in `opts` is ignored with a
notice. When a bind would otherwise run, disabling it (or its group) consumes
the key without calling the callback.

```lua
rune.bind("f1", function() rune.send("north") end, {group = "combat"})
```

To extend a default rather than discard it, capture its action first:

```lua
local scroll = assert(rune.binds.get("pgup")):action()
rune.bind("pgup", function()
    scroll()
    rune.echo("scrolled")
end)
```

## Key format

`key` is a canonical Rune key name such as `"j"`, `"ctrl+r"`, `"pgup"`, or
`"f13"`. The key-name table, modifier order, terminal caveats, and reserved keys
are documented under [Key names](/scripting/keybindings/#key-names).
Aliases are not normalized: use `esc`, `pgup`, and `pgdown`, not `escape`,
`pageup`, or `pagedown`.

## Dispatch behavior and defaults

The Keybindings guide is the canonical description of
[where binds run](/scripting/keybindings/#where-binds-run) and the
[default keymap](/scripting/keybindings/#defaults). It distinguishes normal
input, inline and modal pickers, scrollback search, and the composer.

## Managing

Standard registry management applies, addressed by key:
`rune.binds.get/enable/disable/remove(key)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)`. See
[Registries](/reference/api/#managing). `/binds` lists everything.

**Related:** [Keybindings guide](/scripting/keybindings/) ·
[rune.input](/reference/api/input/) ·
[rune.ui.picker](/reference/api/picker/) ·
[rune.pane](/reference/api/pane/)
