---
title: rune.ui.picker
description: Full signatures for the fuzzy-filter selection panel — options, item formats, and modes.
---

The picker is a fuzzy-filtering selection panel over a list of items —
the machinery behind the world picker, history search, and slash
command autocomplete. For a task-oriented introduction, see
[Pickers](/interface/pickers/).

## Quick reference

```lua
rune.ui.picker.show(opts)  -- open a picker overlay
```

### rune.ui.picker.show

```lua
rune.ui.picker.show(opts)
```

- `title` (string, optional) — header text; modal mode only.
- `items` (array) — the choices; see [Item formats](#item-formats).
- `on_select` (function) — `function(value)`; called with the chosen
  item's value.
- `mode` (string, optional) — `"modal"` (default) or `"inline"`.
- `match_description` (bool, optional) — include item descriptions in
  the fuzzy match.
- `dismiss_on_space` (bool, optional) — inline mode: close the picker
  once the input contains a space. For pickers over single-token items
  (slash commands), where a space means the user has committed and is
  typing arguments.

## Item formats

Plain strings (text and value are the same):

```lua
items = {"north", "south", "east", "west"}
```

Or tables with fields:

```lua
items = {
    {text = "go north", desc = "Move to the forest", value = "north"},
    {text = "go south", desc = "Move to the town", value = "south"},
}
```

## Modes

**Modal** (default) captures all keyboard input and has its own
search field — a focused selection dialog. The default `ctrl+r`
history search is one:

```lua
rune.bind("ctrl+r", function()
    local history = rune.history.get()
    local items = {}
    for i = #history, 1, -1 do
        table.insert(items, history[i])
    end
    rune.ui.picker.show({
        title = "History",
        items = items,
        on_select = function(val)
            rune.input.set(val)
        end
    })
end)
```

**Inline** filters from the live input field as you type —
autocomplete-style selection. The default `/` command picker runs
inline with `match_description` and `dismiss_on_space` set.

## Navigation

| Key | Action |
|---|---|
| `up` / `down` | Move the selection |
| `enter` / `tab` | Accept the selection |
| `esc` | Cancel |
| Typing | Filter items |

**Related:** [Pickers guide](/interface/pickers/) ·
[rune.input](/reference/api/input/) ·
[rune.bind](/reference/api/bind/)
