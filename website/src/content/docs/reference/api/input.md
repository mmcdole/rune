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

History survives `/reload`. Input hooks run before Rune stores the current
submission. Rune stores the final non-empty text with its original command or
verbatim mode. A canceled submission or an accepted rewrite to `""` is not
added. Arrow navigation and `ctrl+r` restore the stored mode, so even a
one-line verbatim entry returns to the composer. Consecutive entries are
deduplicated only when both their text and mode match.

`get()` returns the text-only view and does not expose the stored mode.
`add(cmd)` adds a normal command entry for scripts that want a synthetic
command (one sent by an alias, say) to be recallable. Because it creates a
normal command entry, `cmd` must be valid command text: ordinary game commands
stay on one line, while local `/commands` may have multiline, tab-indented
arguments. Terminal controls are rejected. Verbatim history entries come only
from submitted verbatim input.

**Related:** [Input & History guide](/interface/input/) ·
[rune.bind](/reference/api/bind/) ·
[rune.ui.picker](/reference/api/picker/)
