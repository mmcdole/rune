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

- In normal input, `enter` submits a Rune command. In the visible composer it
  submits the whole draft verbatim. It is owned by the client and not
  rebindable.
- `ctrl+enter` inserts a newline and enters or continues the composer. Most
  terminals encode this as `ctrl+j`; Rune reserves that key for this input
  mechanic rather than dispatching a Lua bind.
- Bracketed paste is handled atomically before binds. A plain one-line paste
  stays in normal input; structured text enters the composer without firing a
  printable hotkey.
- While a picker is open, `ctrl+c`/`escape` cancel it and other keys
  are captured by the picker.
- While the composer is open, the client owns text editing, cursor movement,
  literal `tab`, and two-step `escape` discard. Unhandled application chords,
  including the default `ctrl+e`, can still reach Lua binds.
- In normal input, bound printable keys (like `"j"`) fire only while the input
  is empty, so hotkeys don't break typing. Non-printable bound keys fire unless
  an active picker or composer owns them.
- Outside those input mechanics, the defaults below are ordinary Lua binds and
  can be rebound or removed.

## Default keymap

All defaults are registered by the Lua core and are rebindable:

| Key | Action |
|---|---|
| `ctrl+r` | History search (modal picker) |
| `ctrl+t` | Alias search (modal picker) |
| `/` | Slash command autocomplete (inline picker) |
| `ctrl+c` | Clear input; on empty input, double-tap to quit |
| `escape` | Clear normal input; in the composer, press twice to discard |
| `ctrl+u` | Clear entire input line |
| `ctrl+w`, `alt+backspace` | Delete previous word |
| `up` / `down` | History navigation (prefix-matching) |
| `alt+left` / `alt+right`, `ctrl+left` / `ctrl+right` | Word navigation |
| `tab` / `shift+tab` | Completion cycling |
| `ctrl+e` | Edit input in `$EDITOR` |
| `pageup` / `pagedown` | Scroll output viewport |
| `ctrl+home` / `ctrl+end` | Jump to top/bottom of output |

Bare `home` / `end` are deliberately not bound: they move the input
cursor to the start or end of the line, the same keymap the composer
uses. Binding them replaces that cursor movement with your callback —
`rune.bind("end", function() rune.pane.scroll_to_bottom("main") end)`
puts the scroll jump back on `end`, useful when your terminal cannot
send distinct `ctrl+home` / `ctrl+end` (tmux without `xterm-keys`,
macOS Terminal.app).

The table describes normal input. In the verbatim composer, `tab` inserts a
literal tab, navigation keys edit or scroll the draft, and `ctrl+u` deletes to
the start of the current physical line. The composer footer shows only the
fixed submit, newline, and discard keys; rebindable actions such as the default
`ctrl+e` editor remain documented in this table.

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
