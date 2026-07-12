---
title: Input & History
description: The normal command line, lossless multiline composer, history, completion, and editing input in $EDITOR.
---

Everything you type goes through the input line at the bottom of the
screen, and it behaves like a modern shell: history that matches what
you've started typing, fuzzy search, tab completion, and word-level
editing. When you paste or compose something with multiple lines, the input
grows into a [multiline composer](#multiline-verbatim-composer) that sends
your text exactly as written. None of it needs configuration.

## Normal command input

Press `Enter` to run the input as a Rune command. Aliases, `;` command
separators, `#N` repeats, and `/commands` all work here.

| Key | Action |
|---|---|
| `Ctrl+U` | Clear the input line |
| `Escape` | Clear the input line |
| `Ctrl+W`, `Alt+Backspace` | Delete the word before the cursor |
| `Alt+Left`/`Alt+Right`, `Ctrl+Left`/`Ctrl+Right` | Move the cursor by word |
| `Ctrl+C` | Clear the input line; pressed twice on an empty line, quit |

(Most terminals send `Ctrl+Backspace` as `Ctrl+H`, so it can't be bound
distinctly — use `Ctrl+W` or `Alt+Backspace` to delete words.)

## Multiline verbatim composer

Paste text that contains newlines or tabs and the input grows into a taller
composer. Nothing is lost on the way in: blank lines, indentation, tabs, and
trailing spaces all survive (CRLF and lone-CR line endings are normalized to
LF). To start a multiline draft by hand, press `Ctrl+Enter`.

While the composer is open, the input shows `VERBATIM` and a line count:

| Key | Action |
|---|---|
| `Enter` | Send the draft |
| `Ctrl+Enter` | Insert a newline |
| `Tab` | Insert a literal tab |
| `Ctrl+E` | Edit the whole draft in `$EDITOR` |
| `Escape` twice | Discard the draft (any other key cancels the first press) |

Verbatim means exactly that: each line goes to the server as written. Aliases
don't expand, `;` doesn't split, `#N` doesn't repeat, and a line starting with
`/` is sent as text rather than run as a slash command. The composer stays in
verbatim mode until you send or discard the draft, even if you edit it back
down to a single plain line.

Inside the composer, `Up`/`Down` move through the draft and
`PageUp`/`PageDown` move a page at a time. When the composer closes, the
one-line input and all your usual key bindings return.

As a guard against runaway pastes, a draft larger than 1,000 lines or 256 KiB
is rejected outright: Rune shows a warning and keeps the draft open so you can
trim it. It never silently truncates what you wrote.

## Edit in $EDITOR

`Ctrl+E` opens the current input in Vim, Emacs, or whatever `$EDITOR` points
at. Save and exit, and the result lands back in the input exactly as you
wrote it — line breaks, indentation, and blank lines included (only the
editor's final trailing newline is dropped). A multiline result opens in the
[composer](#multiline-verbatim-composer), ready to send verbatim. Useful for
mails, notes, and anything else that outgrows a one-line edit.

## History

`Up`/`Down` walk your history, and they prefix-match: with `tell ` already
typed, `Up` cycles only through your previous tells. With an empty line,
they walk everything.

History remembers whether each entry was a command or a verbatim draft, and
recalling a verbatim entry reopens the composer — even a one-line entry.
`Ctrl+R` opens a fuzzy history picker, with verbatim entries labeled: type a
few characters, watch the list narrow, and press `Enter` to restore the match.

Recalled verbatim entries don't trap you in the composer: until you edit the
draft, `Up` from its first line and `Down` from its last line keep walking
history. Once you edit it, the arrows move within the draft.

History is owned by the client, so it survives `/reload`. Consecutive
duplicate submissions are stored once.

## Tab completion

In normal input, `Tab` cycles completions from a cache of words seen in server
output and your own input, so NPC names, item names, and player names complete
after they've appeared once. Completion needs at least two typed characters
and skips words shorter than three. Candidates cycle most-recent-first, shown
in the status bar, with `Shift+Tab` going backward. In the composer, `Tab`
inserts a tab instead.

## Scrolling and the mouse

`PageUp`/`PageDown` scroll the output viewport; `Home`/`End` jump to the top and
bottom. The mouse wheel scrolls too. While you're off the bottom, the status
bar shows `SCROLL (n new)` so you know what's piling up, and it returns to
`LIVE` when you catch up. While the composer is open, those keys move within
the draft instead; the mouse wheel still scrolls output.

The mouse is captured for scrolling, so select text with shift+drag, the
standard convention in terminal apps like tmux.

## The default keymap

Actions like history, completion, and `Ctrl+E` are registered with
`rune.bind` in the core scripts — rebind or remove any of them in your
`init.lua` (see [Key Bindings](/scripting/keybindings/)). A few keys are
fixed by the client and can't be rebound: `Enter` to submit, `Ctrl+Enter`
for composer newlines, paste handling, and the composer's editing keys. The
full policy and default table are in the
[rune.bind reference](/reference/api/bind/#key-policy).

**Related:** [rune.input reference](/reference/api/input/),
[Key Bindings](/scripting/keybindings/) for binding your
own, [Pickers](/interface/pickers/) for the overlay UI behind
`Ctrl+R` and `/`
