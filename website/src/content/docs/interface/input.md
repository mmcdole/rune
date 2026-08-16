---
title: Input & History
description: The normal command line, history, completion, scrolling, the lossless multiline composer, and editing input in $EDITOR.
---

Ordinary commands use the single-line input at the bottom of the screen. It
behaves like a modern shell: prefix-matching history, fuzzy search, tab
completion, and word-level editing.

## Normal command input

Press `Enter` to run the input as a Rune command. Aliases, `;` command
separators, `#N` repeats, and `/commands` all work here.

| Key | Action |
|---|---|
| `Ctrl+U` | Clear the input line |
| `Escape` | Clear the input line |
| `Ctrl+W`, `Alt+Backspace` | Delete the word before the cursor |
| `Alt+Left`/`Alt+Right`, `Ctrl+Left`/`Ctrl+Right` | Move the cursor by word |
| `Home`/`End` | Move the cursor to the start/end of the line |
| `Ctrl+C` | Clear the input line; pressed twice on an empty line, quit |

(On terminals that encode `Ctrl+Backspace` as `Ctrl+H`, Rune cannot bind them
distinctly. Use `Ctrl+W` or `Alt+Backspace` to delete words instead.)

## Keeping the last command

By default the input clears after each submit. With
`rune.config.set("keep_input", true)` in your `init.lua`, the command you typed
stays in the input line, shown selected: press `Enter` to submit it
again, type to replace it, or press `Left` or `Right` to deselect it and edit
in place.
Handy for walking with `n` `Enter` `Enter` `Enter`. While the line is
selected it counts as empty for printable-key binds, so hotkeys keep
firing.

Input hooks can change a command before Rune runs it, but the selected text
still shows exactly what you typed. For example, if `!` expands to `north`,
the input line keeps `!` while Rune runs and saves `north`.

## History

`Up`/`Down` walk submission history, and normal command drafts prefix-match:
with `tell ` already typed, `Up` cycles only through previous entries beginning
with `tell `. With an empty line, they walk everything.

`Ctrl+R` opens a fuzzy history picker; type a few characters, watch the list
narrow, and press `Enter` to restore the match.

History survives `/reload`. Rune saves the final submitted text after history
expansion and input hooks have changed it. Canceled commands and accepted
rewrites to an empty string are not saved. Consecutive copies are stored once;
normal and verbatim history remain distinct.

History expansion works too. With the default history character, `!` (or `!!`)
repeats the last command, and `!prefix` repeats the newest command beginning
with `prefix`. For example, after `kill rat`, type `!k` to attack again. Rune
saves `kill rat` in history, not `!k`.

You can use one of these forms anywhere a complete command can appear. With the
default `;` command separator, `north;!` sends `north` followed by the command
that came before it. Every expansion on the line searches the history that
existed before you pressed `Enter`, so the line cannot accidentally repeat
itself.

Rune searches earlier normal commands. It ignores local `/commands`, verbatim
blocks, and earlier commands that still contain history expansion syntax. If
any expansion has no match, Rune shows a warning and sends none of the line.

A line beginning with `/` is a local Rune command, so Rune does not perform
history expansion anywhere on that line. It also does not perform history
expansion in verbatim input or in text sent by a script with `rune.send`.

If your game uses `!` commands, choose another history character or turn the
feature off:

```lua
rune.config.set("history_character", "^") -- ^, ^^, and ^prefix
rune.config.set("history_character", "")  -- disabled
```

`history_character` accepts one visible character, such as `!`, `^`, or `§`.
Use an empty string to disable history expansion.

## Tab completion

In normal input, `Tab` cycles completions from a cache of words seen in server
output and your own input, so NPC names, item names, and player names complete
after they've appeared once. Completion needs at least two typed characters
and skips words shorter than three. Candidates cycle most-recent-first, shown
in the status bar, with `Shift+Tab` going backward.

## Scrolling and the mouse

`PageUp`/`PageDown` scroll the output viewport; `Ctrl+Home`/`Ctrl+End` jump to
the top and bottom (`Home`/`End` stay on the input line; rebind them if you
prefer they scroll). While you're off the bottom, the status bar shows
`SCROLL (n new)` so you know what's piling up, and it returns to `LIVE` when
you catch up.

Rune leaves the mouse to the terminal by default, so click-drag selects text
normally. To capture it for wheel scrolling, enable the `mouse` setting:

```lua
rune.config.set("mouse", true)
```

The change takes effect immediately and does not require a restart. Put it in
`init.lua` to enable it on each start. While capture is enabled, each wheel tick
scrolls three lines and most terminals require holding `Shift` while dragging
for native text selection.

## Multiline verbatim composer

A plain one-line paste stays in normal input and adds no mode label or other
chrome. Pasting structured text switches the input area instead to a taller
composer. Newlines, tabs, blank lines, indentation, trailing spaces, and
terminal control bytes are kept in the draft; CRLF and bare CR line endings are
normalized to LF. You can also press `Ctrl+Enter` to insert the first newline
and enter the composer.

The composer displays `VERBATIM` and its physical line count, so its submit
behavior is explicit:

| Key | Action |
|---|---|
| `Enter` | Send the draft verbatim |
| `Ctrl+Enter` | Insert a newline |
| `Tab` | Insert a literal tab |
| `Ctrl+E` | Edit the whole draft in `$EDITOR` |
| `Escape` twice | Discard the draft; any other key cancels the first warning |

Verbatim submission treats LF, CRLF, and bare CR as line breaks and sends each
physical line without command processing. Rune does not expand aliases, split
at the configured command separator, apply `#N` repeats, or interpret `/quit`
and other slash-looking lines. Those are all data. The mode remains verbatim
after edits even if you remove the last newline or tab; it ends when you send
or discard the draft.

Composer editing keys are handled locally rather than by Lua binds. `Up`/`Down`
move through the draft's visual rows, `PageUp`/`PageDown` move by a composer
page, and the mouse wheel still scrolls output when mouse capture is enabled.
The ordinary one-line input and its bindings return after the composer closes.

So that an accidental paste can't flood the connection, a verbatim submission
is capped at 1,000 physical lines and 256 KiB; over either limit Rune rejects
the whole submission, leaves the draft open, and warns, rather than silently
truncating.

Recalling a verbatim entry from history restores the composer, even when that
entry contains only one physical line. History retains both the text and the
mode it was submitted in, and `Ctrl+R` labels verbatim entries. For an
unmodified restored entry, `Up` on its first visual row and `Down` on its last
continue through history instead of trapping navigation inside the composer.
Once you change its text, the arrows remain local to the draft.

## Edit in $EDITOR

`Ctrl+E` opens the current input in Vim, Emacs, or whatever `$EDITOR` points
at. Save and exit, and the edited result replaces the input without converting
newlines into command separators or trimming whitespace you wrote. Rune
normalizes CRLF and bare CR to LF and removes exactly one final LF used as the
text file terminator. Additional blank lines, indentation, tabs, trailing
spaces, and an intentionally empty result are preserved.

A structured editor result enters the visible verbatim composer. Editing an
existing composer keeps it verbatim even if the result is now one non-empty
line.

## The default keymap

Application actions such as history, completion, and `Ctrl+E` are registered
with `rune.bind` in the core scripts and can be rebound or removed in
`init.lua`. Paste handling, composer editing, `Ctrl+Enter`, and submitting with
`Enter` keep their built-in behavior. The full policy and default table are in
the [Key Bindings guide](/scripting/keybindings/#where-binds-run).

**Related:** [rune.input reference](/reference/api/input/),
[Key Bindings](/scripting/keybindings/) for binding your
own, [Pickers](/interface/pickers/) for the overlay UI behind
`Ctrl+R` and `/`
