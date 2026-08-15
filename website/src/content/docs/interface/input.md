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

(Most terminals send `Ctrl+Backspace` as `Ctrl+H`, so it can't be bound
distinctly — use `Ctrl+W` or `Alt+Backspace` to delete words.)

## Keeping the last command

By default the input clears after each submit. With
`rune.config.set("keep_input", true)` in your `init.lua`, the authored command
stays in the input line, shown selected: press `Enter` to submit it
again, type to replace it, or use the arrow keys to edit it in place.
Handy for walking with `n` `Enter` `Enter` `Enter`. While the line is
selected it counts as empty for printable-key binds, so hotkeys keep
firing.

Input hooks may rewrite what is actually recorded, echoed, and dispatched;
the selected text remains what you authored. For example, if `!` expands to
`north`, the input still shows `!` while the effective submission is `north`.

## History

`Up`/`Down` walk submission history, and normal command drafts prefix-match:
with `tell ` already typed, `Up` cycles only through previous entries beginning
with `tell `. With an empty line, they walk everything.

`Ctrl+R` opens a fuzzy history picker; type a few characters, watch the list
narrow, and press `Enter` to restore the match.

History is owned by the client, so it survives `/reload`. Input hooks run
before a submission is stored and therefore see only earlier entries. A
successful rewrite stores the effective text; a submission consumed with
`false` is not stored. Consecutive identical effective submissions in the same
mode are stored once.

Shell-style history expansion works too: `!` (or `!!`) resends the last
command, and `!prefix` resends the newest command starting with `prefix`, so
`!k` after `kill rat` attacks again. As in the shells, history records the
expanded command, not the bang line.

Expansion is the named input hook `history-expansion` at priority 100. Each
complete component separated by the configured delimiter can expand, so
`north;!` sends `north` and then the previously recorded command. All
components resolve from the same prior-history snapshot. Eligible candidates
come from command-mode entries, but slash commands and entries that still
contain a bang designator are skipped; verbatim entries are skipped as well.
If any designator has no match, Rune warns and consumes the entire submission
without echoing, recording, or dispatching it.

A submission whose first byte is `/` bypasses expansion, so `!` remains literal
inside local-command arguments. Delimiter recognition also happens before
designator recognition: if the configured delimiter itself contains `!`, bytes
used as part of that delimiter are separators, not history designators.

History expansion applies only to interactive command input. Verbatim input
bypasses it, and `rune.send("!")` sends a literal bang. Disable expansion on a
game where `!` is a command with:

```lua
rune.hooks.remove("history-expansion")
```

## Tab completion

In normal input, `Tab` cycles completions from a cache of words seen in server
output and your own input, so NPC names, item names, and player names complete
after they've appeared once. Completion needs at least two typed characters
and skips words shorter than three. Candidates cycle most-recent-first, shown
in the status bar, with `Shift+Tab` going backward.

## Scrolling and the mouse

`PageUp`/`PageDown` scroll the output viewport; `Ctrl+Home`/`Ctrl+End` jump to
the top and bottom (`Home`/`End` stay on the input line; rebind them if you
prefer they scroll). The mouse wheel scrolls too. While you're off the bottom,
the status bar shows `SCROLL (n new)` so you know what's piling up, and it
returns to `LIVE` when you catch up.

The mouse is captured for scrolling, so select text with shift+drag, the
standard convention in terminal apps like tmux.

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
semicolons, apply `#N` repeats, or interpret `/quit` and other slash-looking
lines. Those are all data. The mode remains verbatim after edits even if you
remove the last newline or tab; it ends when you send or discard the draft.

Composer editing keys are handled locally rather than by Lua binds. `Up`/`Down`
move through the draft's visual rows, `PageUp`/`PageDown` move by a composer
page, and the mouse wheel still scrolls output. The ordinary one-line input and
its bindings return after the composer closes.

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
newlines into command separators or trimming authored whitespace. Rune
normalizes CRLF and bare CR to LF and removes exactly one final LF used as the
text file terminator. Additional blank lines, indentation, tabs, trailing
spaces, and an intentionally empty result are preserved.

A structured editor result enters the visible verbatim composer. Editing an
existing composer keeps it verbatim even if the result is now one non-empty
line.

## The default keymap

Application actions such as history, completion, and `Ctrl+E` are registered
with `rune.bind` in the core scripts and can be rebound or removed in
`init.lua`. Input mechanics — atomic paste, composer editing, `Ctrl+Enter`, and
`Enter` submission — are owned by the client. The full policy and default table
are in the [rune.bind reference](/reference/api/bind/#key-policy).

**Related:** [rune.input reference](/reference/api/input/),
[Key Bindings](/scripting/keybindings/) for binding your
own, [Pickers](/interface/pickers/) for the overlay UI behind
`Ctrl+R` and `/`
