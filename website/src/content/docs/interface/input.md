---
title: Input & History
description: The normal command line, history, completion, scrolling, the lossless multiline composer, and editing input in $EDITOR.
---

The single-line input supports command history, fuzzy search, tab completion,
and word-level editing.

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

### Command separators

Use `;` to separate commands, or double it to send a literal semicolon:

```text
look;north             → two commands
say hello;;friend      → say hello;friend
```

Aliases receive the literal separator too: if you define a custom `queue`
alias, `queue look;;north` passes `look;north` as its arguments. History keeps
the doubled spelling so recalling the command behaves the same way.

If you configure another `command_separator`, double that entire separator
instead (`||` for `|`, or `::::` for `::`). Pairs are read left to right:
`;;;` is a literal semicolon followed by a command boundary. To include an
empty command between separators, use `; ;` instead of `;;`.

### Repeating commands

`#3 north` sends `north` three times. Repeats apply to one command or alias:
`#3 north;look` repeats `north`, then sends `look` once. To repeat a sequence,
define an [alias](/scripting/aliases/#examples) and use `#3 aliasname`, or write
a Lua loop.

## Keeping the last command

By default the input clears after each submit. With
`rune.config.set("keep_input", true)` in your `init.lua`, the command you typed
stays in the input line, shown selected: press `Enter` to submit it
again, type to replace it, or press `Left` or `Right` to deselect it and edit
in place.
For example, type `n` and press `Enter` repeatedly to walk north. A selected
line counts as empty for printable-key bindings.

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
`SCROLL (n new)` with the number of lines received while scrolled. It returns
to `LIVE` at the bottom.

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

A plain one-line paste stays in normal input. Pasting structured text opens
the multiline composer. Newlines, tabs, blank lines, indentation, trailing spaces, and
terminal control bytes are kept in the draft; CRLF and bare CR line endings are
normalized to LF. You can also press `Ctrl+Enter` to insert the first newline
and enter the composer.

The multiline composer displays `COMMAND` or `VERBATIM` and its physical line
count on the left, with `Alt+V command` or `Alt+V verbatim` on the right.
The footer shows Enter to submit and `Ctrl+J newline` in both modes.
Verbatim also shows `Alt+Enter run`; `Esc×2 discard` describes the two-press
discard action. When space permits, `Ctrl+E editor` appears if its binding
is present. Narrow layouts omit secondary hints and then the line count
to keep essential actions visible; hints are never cut mid-label.

Ordinary single-line input keeps plain borders. Pressing `Alt+V` opens the
composer in Verbatim mode, preserving text, cursor, and selection. The composer
stays visible when you switch modes or remove the last newline; an accepted
submission or discard closes it unless `keep_input` retains the command.
The editor and interpretation are independent: multiline text can be a command,
and a single line can be sent verbatim.

| Key | Action |
|---|---|
| `Alt+V` | Toggle Command/Verbatim without changing text, cursor, or selection |
| `Enter` | Submit using the displayed mode |
| `Alt+Enter` | Run this draft as a command once, without changing its mode |
| `Ctrl+Enter` or `Ctrl+J` | Insert a newline |
| `Tab` in the composer | Insert a literal tab |
| `Ctrl+E` | Edit the whole draft in `$EDITOR` |
| `Escape` twice in the composer | First press shows `Esc again to discard`; the second discards the draft |

A different key cancels discard confirmation and performs its usual action;
for example, Enter still submits.

Structured paste initially chooses Verbatim. Once you explicitly switch modes,
your choice stays with that draft through edits, additional pastes, and external
editor changes. Removing the last newline does not switch modes. A fresh draft
starts in Command mode.

Verbatim submission treats LF, CRLF, and bare CR as line breaks and sends each
physical line without command processing. Aliases, command separators, `#N`
repeats, and slash-looking lines such as `/quit` are all literal data.

Command mode interprets commands and aliases. A multiline draft must be one
local `/command`: its arguments retain their newlines and tabs. For example,
paste this, then press `Alt+Enter` (or `Alt+V`, then `Enter`):

```lua
/lua -- this comment ends at the newline
local timer = rune.timer.after(60, function() rune.echo("Timer fired") end)
rune.echo("Timer created")
```

Rune passes the whole Lua source to `/lua`; it does not join lines or execute
each line as a separate command. Ordinary game commands must stay on one line;
use the configured command separator for a command sequence. Command mode
rejects terminal control characters. A rejected submission leaves your draft
and its mode intact so you can edit it or switch to Verbatim.

Composer editing keys are handled locally rather than by Lua binds. `Up`/`Down`
move through the draft's visual rows, `PageUp`/`PageDown` move by a composer
page, and the mouse wheel still scrolls output when mouse capture is enabled.
The ordinary one-line input and its bindings return after the composer closes.

A submission in either mode is limited to 1,000 physical lines and 256 KiB. If either
limit is exceeded, Rune rejects the submission, leaves the draft open, and
shows a warning.

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

A structured editor result opens the composer and initially selects Verbatim.
An explicit mode choice stays in effect, including Command mode for multiline
Lua. Replacing a draft with one non-empty line preserves its mode.

## The default keymap

Application actions such as history, completion, and `Ctrl+E` are registered
with `rune.bind` in the core scripts and can be rebound or removed in
`init.lua`. Paste handling, composer editing, mode switching with `Alt+V`,
`Ctrl+Enter`/`Ctrl+J`, and submission with `Enter`/`Alt+Enter` keep their built-in behavior. The full policy and default table are in
the [Key Bindings guide](/scripting/keybindings/#where-binds-run).

**Related:** [rune.input reference](/reference/api/input/),
[Key Bindings](/scripting/keybindings/) for binding your
own, [Pickers](/interface/pickers/) for the overlay UI behind
`Ctrl+R` and `/`
