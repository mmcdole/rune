---
title: rune.bind
description: Full signatures for key bindings — key formats, the key policy, and the default keymap.
---

Key bindings run Lua callbacks on key presses. For a task-oriented
introduction, see [Keybindings](/scripting/keybindings/).

## Quick reference

```lua
rune.bind(key, callback, opts?)   -- bind a key; rebinding replaces (upsert by key)
rune.unbind(key)                  -- remove a binding; true if one existed
```

`rune.bind` returns a [handle](/reference/api/#handles) and accepts the
[common options](/reference/api/#options) `name` and `group`. A disabled
bind (or one in a disabled group) swallows its key without running the
callback.

```lua
rune.bind("f1", function() rune.send("north") end, {name = "go-north"})
```

## Key formats

| Format | Examples |
|---|---|
| Single character | `"j"`, `"/"`, `"."` |
| Ctrl combinations | `"ctrl+r"`, `"ctrl+t"`, `"ctrl+a"` |
| Alt combinations | `"alt+left"`, `"alt+backspace"`, `"alt+x"` |
| Function keys | `"f1"` through `"f12"` |
| Navigation | `"up"`, `"down"`, `"left"`, `"right"`, `"pageup"`, `"pagedown"`, `"home"`, `"end"`, `"escape"`, `"tab"`, `"shift+tab"` |

## Key policy

- `enter` always submits input (owned by the client, not rebindable).
- While a picker is open, `ctrl+c`/`escape` cancel it and other keys
  are captured by the picker.
- Bound printable keys (like `"j"`) fire only while the input line is
  empty, so hotkeys don't break typing. Non-printable bound keys
  always fire.
- Everything else — including all the defaults below — is an ordinary
  Lua bind and can be rebound or removed.

## Default keymap

All defaults are registered by the Lua core and are rebindable:

| Key | Action |
|---|---|
| `ctrl+r` | History search (modal picker) |
| `ctrl+t` | Alias search (modal picker) |
| `/` | Slash command autocomplete (inline picker) |
| `ctrl+c` | Clear input; on empty input, double-tap to quit |
| `escape` | Clear input |
| `ctrl+u` | Clear entire input line |
| `ctrl+w`, `alt+backspace` | Delete previous word |
| `up` / `down` | History navigation (prefix-matching) |
| `alt+left` / `alt+right`, `ctrl+left` / `ctrl+right` | Word navigation |
| `tab` / `shift+tab` | Completion cycling |
| `ctrl+e` | Edit input in `$EDITOR` |
| `pageup` / `pagedown` | Scroll output viewport |
| `home` / `end` | Jump to top/bottom of output |

Most terminals send `ctrl+backspace` as `ctrl+h`, so it cannot be
bound distinctly from `ctrl+h`; use `ctrl+w` or `alt+backspace` for
delete-word.

## Managing

Standard registry management applies:
`rune.binds.enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/binds` lists everything.

**Related:** [Keybindings guide](/scripting/keybindings/) ·
[rune.input](/reference/api/input/) ·
[rune.ui.picker](/reference/api/picker/) ·
[rune.pane](/reference/api/pane/)
