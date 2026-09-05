---
title: rune.ui.picker
description: Full signatures for the fuzzy-filter selection panel — options, item formats, and modes.
---

The picker filters a list of items as you type. World selection, history,
aliases, and slash-command completion use this control. For examples, see
[Pickers](/interface/pickers/).

## Quick reference

```lua
rune.ui.picker.show(opts)  -- open a picker in the input area
```

### rune.ui.picker.show

```lua
rune.ui.picker.show(opts)
```

- `title` (string, optional) — label beside the filter field; modal mode only.
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

**Modal** (default) captures keyboard input and shows results above a labeled
filter field. The command draft is hidden until the picker closes.
For example, open a history picker with `ctrl+r`:

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

**Inline** shows suggestions above the command field and filters them as you
type into that field. The default `/` command picker runs
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
