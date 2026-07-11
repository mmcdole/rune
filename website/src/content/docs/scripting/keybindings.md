---
title: Key Bindings
description: Bind keys and chords to Lua callbacks. The default keymap is a script too.
---

```lua
rune.bind("f1", function() rune.send("cast shield") end)
rune.bind("ctrl+g", function() rune.pane.toggle("map") end, { name = "map-toggle" })
rune.unbind("f1")
```

The callback is always a function; binds don't take command strings the way
aliases and triggers do. Call `rune.send` inside the callback.

## Key names

Printable keys use the character itself: `a`-`z`, digits, `` ` ``, and so
on. Special keys: `f1`-`f12`, `up/down/left/right`,
`pageup/pagedown`, `home/end`, `tab`, `escape`, `backspace`, `delete`,
`insert`. Modifiers: `alt+` combines with any key; `ctrl+` with letters,
arrows, `pageup/pagedown`, and `home/end`; `shift+` with `tab`, arrows, and
`home/end`. Enter is not bindable: it submits a normal command or, while the
visible composer is open, sends the draft verbatim. `Ctrl+Enter` (reported by
most terminals as `ctrl+j`) is reserved for inserting a composer newline.

## Printable keys and typing

A bound printable key (like `` ` `` or `j`) fires only when the input line
is empty, so hotkeys and typing coexist without a modal system. Type `jump`
normally; press `j` on an empty line and it acts as a hotkey.

Bracketed paste is also intercepted before binds, so pasting one bound
character cannot trigger it. A plain one-line paste stays in normal input;
structured text enters the [verbatim composer](/interface/input/#multiline-verbatim-composer).

## Internal input modes

Pickers and the composer own the keys needed to edit or cancel them. In the
composer that includes text and cursor editing, literal `Tab`, and two-step
`Escape` discard. Application chords the composer does not use can still run a
Lua bind — the default `Ctrl+E` editor binding is the important example. When
the composer or picker closes, normal binding policy resumes.

## Options

Binds take the [common options](/scripting/model/#options) `name` and
`group`. Rebinding a key always replaces whatever was on it, named or
not.

## Examples

Movement keys, grouped:

```lua
rune.bind("up",    function() rune.send("north") end, { group = "numpad-walk" })
rune.bind("down",  function() rune.send("south") end, { group = "numpad-walk" })
-- /group numpad-walk off  when you need arrows for history again
```

Editing helpers using the input API:

```lua
rune.bind("ctrl+u", function() rune.input.set("") end)
rune.bind("ctrl+w", function() rune.input.delete_word() end)
```

## Defaults

The default keymap (history navigation, pickers, completion, scrolling,
`$EDITOR` editing) is registered with `rune.bind` in the core scripts;
the full table is in the [rune.bind reference](/reference/api/bind/#default-keymap).
Rebinding a key in your `init.lua` replaces the default.

## Managing

By name: `rune.binds.disable/enable/remove(name)` — the full management
suite is in the [API reference](/reference/api/#managing). In the client,
`/binds` lists every binding with its state, group, and the `file:line`
that registered it.

## Gotchas

- A disabled bind (or one in a disabled group) swallows its key without
  running the callback in normal input; the key does not fall through to
  typing. Use `rune.unbind(key)` to give the key back to the input line.
- A callback that errors three times in a row is
  [quarantined](/scripting/model/#quarantine).

**Related:** [rune.bind reference](/reference/api/bind/),
[Input & History](/interface/input/),
[Pickers](/interface/pickers/),
[Groups](/scripting/groups/)
