---
title: rune.input
description: Full signatures for reading and editing the input buffer, plus submission history.
---

Read and modify the input buffer from scripts — pickers, binds, and
completion are all built on these. For the interactive side (default
keys, the multiline composer, history navigation, and tab completion), see
[Input & History](/interface/input/).

## Quick reference

```lua
rune.input.get()                  -- current input text
rune.input.set(text)              -- replace the input text
rune.input.get_cursor()           -- zero-based UTF-8 byte offset
rune.input.set_cursor(pos)        -- move to a UTF-8 byte offset
rune.input.open_editor(initial?)  -- edit in $EDITOR; returns edited_text, ok
rune.input.word_left()            -- move cursor to the previous word boundary
rune.input.word_right()           -- move cursor to the next word boundary
rune.input.delete_word()          -- delete the word before the cursor
```

`get`/`set` operate on the whole buffer; the word operations combine
them with cursor moves and are what the default `ctrl+w`,
`alt+left`/`alt+right` binds call. The `"input_changed"`
[hook event](/reference/api/hooks/) fires whenever the buffer changes,
including typing, history or completion, `rune.input.set`, and the draft left
after submission.

Cursor positions are zero-based UTF-8 byte offsets, using the same byte units
as Lua 5.1 string operations. `set_cursor` clamps positions to the input and
snaps an offset inside a multibyte sequence to the preceding UTF-8 code point
boundary.

Setting text containing a newline, tab, or terminal control byte activates the
visible composer and initially selects Verbatim. An explicit mode choice made
with `Alt+V` or restored from history persists through text changes. Replacing
the draft with one non-empty plain line preserves its mode; setting it to `""`
clears the draft and resets to Command. See
[Multiline verbatim composer](/interface/input/#multiline-verbatim-composer)
for its submission semantics and limits.

## Input bindings

[`rune.bind`](/reference/api/bind/) accepts either a Lua callback or a named
internal action:

```lua
-- Arbitrary Lua behavior
rune.bind("f1", function() rune.send("look") end)

-- Internal action, resolved in the current input context
rune.bind("enter", "input.submit")
rune.bind({"ctrl+j", "shift+enter", "ctrl+enter"}, "input.newline")
```

| Internal action | Behavior | Default keys |
|---|---|---|
| `input.submit` | Submit the draft using its current Command/Verbatim mode | Enter |
| `input.newline` | Insert a newline, opening the composer if necessary | Ctrl+J, Shift+Enter, Ctrl+Enter |
| `input.toggle_mode` | Switch Command/Verbatim interpretation | Alt+V |
| `input.open_editor` | Edit the current draft in `$EDITOR`; apply a successful result without submitting | Ctrl+E |
| `input.cancel` | Cancel according to the current input context, as below | Esc |

| Cancel context | Behavior |
|---|---|
| Normal input | Clear the draft |
| Multiline composer | First press requests confirmation; a second cancel press discards the draft |
| Inline or modal picker | Close without accepting; preserve the draft |
| Scrollback search | Cancel search and restore the previous view |

Any intervening non-cancel key or binding update dismisses multiline discard
confirmation. Rebinding cancel changes it across all these contexts. Ctrl+C also
remains an overlay interrupt; outside overlays it keeps its existing Lua binding.
Modal pickers and search capture other actions. Printable bindings retain the
input contexts' typing protection; prefer non-printable keys for editor actions.

A new key adds an alias; an existing key replaces its assignment. To move an
action, bind the new key and explicitly unbind the old one:

```lua
rune.bind("f2", "input.open_editor")
rune.unbind("ctrl+e")
rune.bind("ctrl+g", "input.cancel")
rune.unbind("esc")
```

Arrays create independent bindings and return an array of handles. A single key
returns one handle. Removing one alias leaves the others intact. Disabling a
binding or its group prevents its action. Defaults are ordinary registrations;
unbinding them does not reveal a hidden default.

Multiline hints use the earliest registered active binding for each action;
search uses the same cancel binding for its hint. If no active binding remains,
its hint disappears. Shift+Enter and Ctrl+Enter require distinct terminal key
reporting; Ctrl+J is the portable newline alternative.

Callbacks execute through Session and Lua. Internal actions are resolved by the
TUI; submission proceeds to Session for processing, and opening the external
editor asks Session to suspend the terminal and apply the edited result. No Lua
callback or Lua watchdog is involved in a named editor action. Action strings
are identifiers, not commands to send or Lua expressions; unknown names are errors.

The `input.open_editor` binding edits the current draft automatically. The Lua
function below instead returns text to its caller, which decides how to use it.

### rune.input.open_editor

```lua
rune.input.open_editor(initial?) -> edited_text, ok
```

- `initial` (string, optional) — text to seed the editor buffer with.

Opens `$EDITOR` (falling back to `vi`/`notepad`) on a temp file; the
client suspends until the editor exits. On success, CRLF and bare CR are
normalized to LF and exactly one final LF (the conventional text-file
terminator) is removed. All other authored whitespace — indentation, tabs,
trailing spaces, and additional blank lines — is preserved. The function
returns that text and `true`, including `"", true` for an intentionally empty
file. It returns `"", false` when the editor could not run or exited with an
error.

The default `ctrl+e` bind is a thin wrapper. A multiline result enters the
composer, respecting an explicit mode choice and preserving newlines:

```lua
rune.bind("ctrl+e", function()
    local text, ok = rune.input.open_editor(rune.input.get())
    if ok then
        rune.input.set(text)
    end
end)
```

## rune.history

```lua
rune.history.get()     -- submitted text, oldest first
rune.history.add(cmd)  -- append a normal command entry
```

History survives `/reload`. After processing a submission, Rune stores the
surviving lines together with their original Command or Verbatim mode. Consumed
and invalid lines are omitted. An empty final history string is not added.
Input hooks, echo hooks, and command handlers do not see the current submission
in history while it is running. Explicit `add` calls remain visible immediately.
Arrow navigation and `ctrl+r` restore the block and its stored mode, so even a
one-line Verbatim entry returns to the composer. Consecutive entries are
deduplicated only when both their text and mode match.

`get()` returns the text-only view and does not expose the stored mode.
`add(cmd)` adds a normal command entry for scripts that want a synthetic
command (one sent by an alias, say) to be recallable. Because it creates a
normal command entry, `cmd` may contain several newline-separated commands
and tabs. Invalid UTF-8 and terminal controls are rejected. Verbatim history entries come only
from submitted verbatim input.

**Related:** [Input & History guide](/interface/input/) ·
[rune.bind](/reference/api/bind/) ·
[rune.ui.picker](/reference/api/picker/)
