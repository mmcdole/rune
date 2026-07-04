---
title: rune.input
description: Full signatures for reading and editing the input line, plus the command history buffer.
---

Read and modify the input line from scripts — pickers, binds, and
completion are all built on these. For the interactive side (default
keys, history navigation, tab completion), see
[Input & History](/interface/input/).

## Quick reference

```lua
rune.input.get()                  -- current input text
rune.input.set(text)              -- replace the input text
rune.input.get_cursor()           -- cursor position (clamped to input length)
rune.input.set_cursor(pos)        -- move the cursor
rune.input.open_editor(initial?)  -- edit in $EDITOR; returns edited_text, ok
rune.input.word_left()            -- move cursor to the previous word boundary
rune.input.word_right()           -- move cursor to the next word boundary
rune.input.delete_word()          -- delete the word before the cursor
```

`get`/`set` operate on the whole line; the word operations combine
them with cursor moves and are what the default `ctrl+w`,
`alt+left`/`alt+right` binds call. Setting the input fires the
`"input_changed"` [hook event](/reference/api/hooks/), same as typing.

### rune.input.open_editor

```lua
rune.input.open_editor(initial?) -> edited_text, ok
```

- `initial` (string, optional) — text to seed the editor buffer with.

Opens `$EDITOR` (falling back to `vi`/`notepad`) on a temp file; the
client suspends until the editor exits. Returns the edited text
(whitespace-trimmed) and `true`, or `""` and `false` when the editor
could not run or exited with an error. The default `ctrl+e` bind is a
thin wrapper:

```lua
rune.bind("ctrl+e", function()
    local text, ok = rune.input.open_editor(rune.input.get())
    if ok and text ~= "" then
        rune.input.set((text:gsub("\n", "; ")))
    end
end)
```

## rune.history

```lua
rune.history.get()     -- array of past commands, oldest first
rune.history.add(cmd)  -- append a command (consecutive duplicates ignored)
```

The buffer is Go-owned, so it survives `/reload`. Everything the user
submits lands here automatically; `add` is for scripts that want
synthetic entries (a command sent by an alias, say) to be recallable
with the up arrow and `ctrl+r`.

**Related:** [Input & History guide](/interface/input/) ·
[rune.bind](/reference/api/bind/) ·
[rune.ui.picker](/reference/api/picker/)
