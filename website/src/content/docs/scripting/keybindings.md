---
title: Key Bindings
description: Bind keys and chords to input actions or Lua callbacks. The default keymap is a script too.
---

A bind attaches an input action or Lua callback to a key or chord, so a keypress can do what
would otherwise take a typed command. The default keymap (history, completion,
scrolling, `$EDITOR`) is built from the same function, so anything it binds you
can rebind.

```lua
rune.bind("f1", function() rune.send("cast shield") end)
rune.bind("ctrl+g", function() rune.pane.toggle("map") end)
rune.unbind("f1")
```

A binding accepts a function or a named input action such as `"input.submit"`.
To send a command, call `rune.send` inside a callback. Strings are action names,
not commands to send.

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

- **Kitty keyboard protocol:** identifies the physical key with NumLock on or off.
- **DEC application keypad mode (DECKPAM):** identifies keypad keys, usually
  with NumLock off.

Enable both in `init.lua`:

```lua
rune.config.set("numpad", true)
```

Rune requests both modes; your terminal uses the one it supports.

| Input mode | Known terminals | Required setup |
|---|---|---|
| Kitty protocol | Ghostty, Kitty, Alacritty, foot, iTerm2 | None |
| Kitty protocol | WezTerm | Set `enable_kitty_keyboard = true` |
| Kitty protocol | Windows Terminal | Version 1.25 or newer |
| DEC keypad | macOS Terminal | Enable **Profiles → Advanced → Allow VT100 application keypad mode** |
| DEC keypad | xterm | Launch `xterm -kt vt220` with NumLock off |
| DEC keypad | urxvt | Use NumLock off |
| Not supported | GNOME Terminal, Ptyxis, COSMIC Terminal | These terminals do not preserve the physical number-pad key |

If numpad bindings fail inside tmux, test Rune directly in the terminal to
check whether tmux is forwarding the keypad information.

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

<span id="editor-action-keys"></span>

### Input action keys

| Key | Normal input | Draft editor |
|---|---|---|
| `enter` | Submit the command | Submit using the displayed mode |
| `alt+v` | Open the draft editor in Verbatim mode | Toggle Command/Verbatim |
| `ctrl+j`, `shift+enter`, `ctrl+enter` | Start a draft editor newline | Insert a newline |
| `ctrl+e` | Edit the draft in `$EDITOR` | Edit the draft in `$EDITOR` |
| `esc` | Clear the draft | Confirm, then discard on the second press |

These are ordinary bindings to named input actions:

```lua
rune.bind("enter", "input.submit")
rune.bind({"ctrl+j", "shift+enter", "ctrl+enter"}, "input.newline")
rune.bind("alt+v", "input.toggle_mode")
rune.bind("ctrl+e", "input.open_editor")
rune.bind("esc", "input.cancel")
```

A new key adds an alias; rebinding an existing key replaces its assignment.
To move submit to Ctrl+S, remove Enter explicitly:

```lua
rune.bind("ctrl+s", "input.submit")
rune.unbind("enter")
```

Arrays create independent bindings and return an array of handles. Removing one
alias leaves the others intact. Defaults load before user scripts, so removal
also works for defaults; there is no hidden default underneath an unbound key.

Draft editor hints show the earliest registered active binding for each action.
Removing or disabling that binding promotes the next alias. An action with no
active bindings has no hint. To prefer Shift+Enter while keeping Ctrl+J, rebind
Ctrl+J after the default Shift+Enter binding:

```lua
rune.bind("ctrl+j", "input.newline")
```

Submit always uses the displayed mode. Alt+Enter has no built-in override.
Modified Enter requires terminal support; Ctrl+J remains the portable alternative.
The cancel action works in every input context; modal pickers and search capture
other actions. See the [complete action table](/reference/api/input/#input-bindings). Inline pickers close
when toggling mode or inserting a newline; Enter still accepts a selection when
submit is rebound. The UI resolves named actions, with Session handling submission and external-editor
requests. Callbacks run through Lua. An explicit
physical keypad-Enter binding wins over the ordinary Enter binding, including
modified keypad Enter.

## Where binds run

| Context | Rune handles locally | Lua binds |
|---|---|---|
| Normal input | Configured input actions; paste is atomic | Non-printable binds run. A printable bind runs only when the input is empty or fully selected; otherwise the character is typed |
| Inline picker | Configured cancel or `ctrl+c` closes; `up`/`down` navigate; `tab` accepts; `enter` accepts; configured submit accepts and submits; newline starts the draft editor; editor closes the picker and edits the draft; unbound text filters | Any other bound key runs, including printable keys |
| Modal picker | Configured cancel closes; all other keys stay in the picker | None |
| Scrollback search | Configured cancel closes; all other keys stay in search | None |
| Draft editor | Text entry, editing and navigation, literal `tab`, submit, newline, external editor, and two-step configured cancel | Unused chords can run |

This lets a printable hotkey coexist with typing: type `jump` normally, but
press a bound `j` on an empty line and its callback runs. A fully selected
kept command also counts as empty because the next typed character would
replace it.

Bracketed paste never runs a bind. Normal input, an inline picker, and the
draft editor insert it all at once, so a bind can't fire partway through it;
structured paste opens the
[draft editor](/interface/input/#multiline-draft-editor).
Verbatim is the initial interpretation unless you explicitly chose a mode for
this draft. Modal pickers and scrollback search append paste to their query.

## Options

Binds take the [common option](/scripting/model/#options) `group`. The
key is the bind's name, so rebinding a key always replaces whatever was
on it, and `rune.binds.disable("ctrl+g")` addresses it by the same
string you bound.

To extend a default instead of discarding it, capture its action first.
`rune.binds.get(key)` returns the handle; `:action()` is the raw
callback or named action string. This example wraps a callback:

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
that movement with your callback; the draft editor continues to own both keys.

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
