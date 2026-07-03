---
title: Input & History
description: The typing experience, including line editing, prefix-matching history, tab completion, and editing the input in $EDITOR.
---

Everything you type goes through one input line at the bottom of the
screen, and it behaves like a modern shell: history that matches what
you've started typing, fuzzy search, tab completion, and word-level
editing. None of it needs configuration, and all of it can be rebound.

## Editing keys

| Key | Action |
|---|---|
| `Ctrl+U` | Clear the input line |
| `Escape` | Clear the input line |
| `Ctrl+W`, `Alt+Backspace`, `Ctrl+Backspace` | Delete the word before the cursor |
| `Alt+Left`/`Alt+Right`, `Ctrl+Left`/`Ctrl+Right` | Move the cursor by word |
| `Ctrl+C` | Clear the input line; pressed twice on an empty line, quit |

## Edit in $EDITOR

`Ctrl+E` opens whatever is on the input line in your own editor: Vim,
Emacs, or whatever `$EDITOR` points at. Save and exit, and the result
lands back on the input line, ready to send. Useful for long say/tell
compositions or fiddly command sequences that outgrow a single-line edit.

## History

`Up`/`Down` walk your command history, and they prefix-match: with `tell `
already typed, `Up` cycles only through your previous tells. With an empty
line, they walk everything.

`Ctrl+R` opens a fuzzy history picker: type a few characters, watch the
list narrow, press `Enter` to put the match on the input line.

History is owned by the client, so it survives `/reload`. Consecutive
duplicate commands are stored once.

## Tab completion

`Tab` cycles completions from a cache of words seen in server output and
your own input, so NPC names, item names, and player names complete after
they've appeared once. Completion needs at least two typed characters and
skips words shorter than three. Candidates cycle most-recent-first, shown
in the status bar, with `Shift+Tab` going backward.

## Scrolling and the mouse

`PageUp`/`PageDown` scroll the output viewport; `Home`/`End` jump to the
top and bottom. The mouse wheel scrolls too. While you're off the bottom,
the status bar shows `SCROLL (n new)` so you know what's piling up, and it
returns to `LIVE` when you catch up.

The mouse is captured for scrolling, so select text with shift+drag, the
standard convention in terminal apps like tmux.

## The default keymap

Every binding below is registered with `rune.bind` in the core scripts.
Rebind or remove any of them in your `init.lua`; see
[Key Bindings](/scripting/keybindings/). `Enter` is the one fixed
key: it always submits the input line.

| Key | Action |
|---|---|
| `Up` / `Down` | History navigation (prefix-matching) |
| `Ctrl+R` | History search (picker) |
| `Ctrl+T` | Alias search (picker) |
| `/` | Slash command picker (on an empty line) |
| `Tab` / `Shift+Tab` | Completion cycling |
| `Ctrl+E` | Edit input in `$EDITOR` |
| `Ctrl+U`, `Escape` | Clear the input line |
| `Ctrl+W`, `Alt+Backspace`, `Ctrl+Backspace` | Delete previous word |
| `Alt+Left`/`Alt+Right`, `Ctrl+Left`/`Ctrl+Right` | Word navigation |
| `PageUp` / `PageDown` | Scroll output |
| `Home` / `End` | Jump to top/bottom of output |
| `Ctrl+C` | Clear input; twice on an empty line, quit |

**Related:** [Key Bindings](/scripting/keybindings/) for binding your
own, [Pickers](/interface/pickers/) for the overlay UI behind
`Ctrl+R` and `/`
