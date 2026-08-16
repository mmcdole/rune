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

The common key forms are below. Use the same exact name in `rune.bind`,
`rune.unbind`, and `rune.binds` methods that take a key. Named keys and modifier
prefixes are lowercase.

| Kind | Names | Examples |
|---|---|---|
| Printable | The character itself; use `space` for a space | `"j"`, `"/"`, `"."`, `"space"` |
| Editing | `esc`, `tab`, `backspace`, `delete`, `insert` | `"esc"`, `"shift+tab"`, `"alt+backspace"` |
| Navigation | `up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdown` | `"left"`, `"ctrl+home"`, `"shift+pgup"` |
| Function keys | `f1` through `f63` | `"f1"`, `"f13"`, `"f20"` |
| Numpad digits | `numpad0` through `numpad9` | `"numpad8"`, `"ctrl+numpad4"` |
| Numpad operators | `numpad_dot`, `numpad_slash`, `numpad_star`, `numpad_minus`, `numpad_plus`, `numpad_enter` | `"numpad_plus"`, `"numpad_enter"` |
| Modifiers | `ctrl`, `alt`, `shift`, `meta`, `hyper`, `super` | `"ctrl+r"`, `"shift+a"`, `"ctrl+alt+x"` |

For multiple modifiers, use the order `ctrl+alt+shift+meta+hyper+super`, then
the base key. Your terminal determines which keys Rune can distinguish. In
particular, extended function keys, modified navigation keys, and
`meta`/`hyper`/`super` chords are not available in every terminal.

Key names are exact. Use `esc`, `pgup`, and `pgdown`; `escape`, `pageup`, and
`pagedown` are not aliases. Numpad names begin with `numpad`; `kp8` and
`kpenter` are not aliases for `numpad8` and `numpad_enter`.

### Numpad keys

Numpad bindings depend on terminal support. If your terminal sends numpad 8 as
an ordinary `8` or `Up`, Rune cannot tell that it came from the number pad.

Rune supports both ways terminals preserve that information:

- **Modern protocol (Kitty keyboard protocol):** identifies the physical key
  with NumLock on or off.
- **Legacy mode (DEC application keypad mode, or DECKPAM):** supported by
  older terminals, usually with NumLock off.

Enable both in `init.lua`:

```lua
rune.config.set("numpad", true)
```

Rune requests both modes; your terminal uses the one it supports.

| Input mode | Known terminals | Required setup |
|---|---|---|
| Modern | Ghostty, Kitty, Alacritty, foot, iTerm2 | None |
| Modern | WezTerm | Set `enable_kitty_keyboard = true` |
| Modern | Windows Terminal | Version 1.25 or newer |
| Legacy | macOS Terminal | Enable **Profiles → Advanced → Allow VT100 application keypad mode** |
| Legacy | xterm | Launch `xterm -kt vt220` with NumLock off |
| Legacy | urxvt | Use NumLock off |
| Not supported | GNOME Terminal, Ptyxis, COSMIC Terminal | These terminals do not preserve the physical number-pad key |

Current tmux releases do not pass the modern protocol through. Some legacy
setups work inside tmux, but test Rune without tmux first.

Bind the physical keys using `numpad0` through `numpad9`:

```lua
rune.bind("numpad8", function() rune.send("north") end)
rune.bind("numpad2", function() rune.send("south") end)
rune.bind("numpad6", function() rune.send("east") end)
rune.bind("numpad4", function() rune.send("west") end)
rune.bind("numpad9", function() rune.send("up") end)
rune.bind("numpad3", function() rune.send("down") end)
```

`numpad8` always means the 8 key on the number pad. The number-row key remains
`8`.

### Reserved input keys

| Key | Normal input | Composer |
|---|---|---|
| `enter` | Submit the command | Send the draft verbatim |
| `ctrl+enter`, `ctrl+j` | Start a composer newline | Insert a newline |

These actions are built in and do not dispatch Lua binds. Terminals that cannot
distinguish Ctrl+Enter report it as Ctrl+J. Picker and search behavior is shown
in the context table below.

## Where binds run

| Context | Rune handles locally | Lua binds |
|---|---|---|
| Normal input | `enter` submits; `ctrl+enter`/`ctrl+j` starts a composer newline; paste is atomic | Non-printable binds run. A printable bind runs only when the input is empty or fully selected; otherwise the character is typed |
| Inline picker | `esc`/`ctrl+c` cancel; `up`/`down` navigate; `tab` accepts; `enter` accepts and submits; `ctrl+enter`/`ctrl+j` starts the composer; unbound text filters | Any other bound key runs, including printable keys |
| Modal picker | All keypresses | None |
| Scrollback search | All keypresses | None |
| Composer | Text entry, editing and navigation, literal `tab`, submit, newline, and two-step `esc` discard | Unused chords can run, including the default `ctrl+e` editor bind |

This lets a printable hotkey coexist with typing: type `jump` normally, but
press a bound `j` on an empty line and its callback runs. A fully selected
kept command also counts as empty because the next typed character would
replace it.

Bracketed paste never runs a bind. Normal input, an inline picker, and the
composer insert it all at once, so a bind can't fire partway through it;
structured paste switches normal or inline input to the
[verbatim composer](/interface/input/#multiline-verbatim-composer).
Modal pickers and scrollback search append paste to their query.

## Options

Binds take the [common option](/scripting/model/#options) `group`. The
key is the bind's name, so rebinding a key always replaces whatever was
on it, and `rune.binds.disable("ctrl+g")` addresses it by the same
string you bound.

To extend a default instead of discarding it, capture its action first.
`rune.binds.get(key)` returns the handle; `:action()` is the raw
callback:

```lua
local scroll = assert(rune.binds.get("pgup")):action()
rune.bind("pgup", function()
    scroll()
    rune.echo("scrolled")
end)
```

## Examples

Movement keys, grouped:

```lua
rune.bind("f5", function() rune.send("north") end, { group = "movement" })
rune.bind("f6", function() rune.send("south") end, { group = "movement" })
-- /group movement off
```

Editing helpers using the input API:

```lua
rune.bind("ctrl+u", function() rune.input.set("") end)
rune.bind("ctrl+w", function() rune.input.delete_word() end)
```

## Defaults

The core scripts register every default through `rune.bind`, so rebinding a key
in your `init.lua` replaces its default action.

| Key | Default action |
|---|---|
| `ctrl+r` | Search command history |
| `ctrl+f` | Search scrollback |
| `ctrl+t` | Search aliases |
| `/` | Open slash-command completion |
| `ctrl+c` | Clear input; on empty input, press twice to quit |
| `esc` | Clear normal input |
| `ctrl+u` | Clear normal input |
| `ctrl+w`, `alt+backspace` | Delete the previous word |
| `up`, `down` | Navigate prefix-matching history |
| `alt+left`, `alt+right`, `ctrl+left`, `ctrl+right` | Move by word |
| `tab`, `shift+tab` | Cycle completion |
| `ctrl+e` | Edit input in `$EDITOR` |
| `pgup`, `pgdown` | Scroll output |
| `ctrl+home`, `ctrl+end` | Jump to the top or bottom of output |

Bare `home` and `end` are deliberately unbound, so they move the input cursor
to the start or end of the line. In normal input, binding either key replaces
that movement with your callback; the composer continues to own both keys.

`pgup`, `pgdown`, `ctrl+home`, and `ctrl+end` also have a built-in fallback, so
output remains scrollable if the Lua defaults are absent. Removing their binds
restores that fallback; disabling a bind consumes its key instead.

On terminals that encode Ctrl+Backspace as `ctrl+h`, Rune cannot distinguish
it from Ctrl+H. Use `ctrl+w` or `alt+backspace` for delete-word there.

## Managing

By key: `rune.binds.get/disable/enable/remove(key)`. The full list is in
the [API reference](/reference/api/#managing). In the client,
`/binds` lists every binding with its state, group, and the `file:line`
that registered it.

## Gotchas

- When a bind would otherwise run, disabling it (or its group) consumes the key
  without calling the callback. Use `rune.unbind(key)` to restore normal
  fallthrough.
- A callback that errors three times in a row is
  [quarantined](/scripting/model/#quarantine).

**Related:** [rune.bind reference](/reference/api/bind/),
[Input & History](/interface/input/),
[Pickers](/interface/pickers/),
[Groups](/scripting/groups/)
