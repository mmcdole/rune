---
title: Key Bindings
description: Bind keys and chords to Lua callbacks. The default keymap is a script too.
---

A bind attaches a Lua callback to a key or chord, so a keypress can do what
would otherwise take a typed command. The default keymap (history, completion,
scrolling, `$EDITOR`) is built from the same function, so anything it binds you
can rebind.

```lua
rune.bind("f1", function() rune.send("cast shield") end)
rune.bind("ctrl+g", function() rune.pane.toggle("map") end)
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

## Options

Binds take the [common option](/scripting/model/#options) `group`. The
key is the bind's name, so rebinding a key always replaces whatever was
on it, and `rune.binds.disable("ctrl+g")` addresses it by the same
string you bound.

To extend a default instead of discarding it, capture its action first.
`rune.binds.get(key)` returns the handle; `:action()` is the raw
callback:

```lua
local scroll = assert(rune.binds.get("pageup")):action()
rune.bind("pageup", function()
    scroll()
    rune.echo("scrolled")
end)
```

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

By key: `rune.binds.get/disable/enable/remove(key)`. The full management
suite is in the [API reference](/reference/api/#managing). In the client,
`/binds` lists every binding with its state, group, and the `file:line`
that registered it.

## When a bind doesn't fire

Pickers and the composer own the keys needed to edit or cancel them, so a bind
on one of those keys goes quiet while they're open. In the composer that covers
text and cursor editing, literal `Tab`, and two-step `Escape` discard.
Application chords the composer does not use still run their Lua bind; the
default `Ctrl+E` editor binding is the important example. Normal binding policy
resumes when the composer or picker closes.

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
