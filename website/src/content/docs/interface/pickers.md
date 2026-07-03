---
title: Pickers
description: Fuzzy-filtered overlays for commands, worlds, and history, including pickers your own scripts can open.
---

## Built-in pickers

- `/` on an empty line opens the **command picker**: every slash command
  (including yours), fuzzy-filtered as you type, with descriptions.
- `/connect` with no arguments opens the **world picker** over your saved
  bookmarks.
- `Ctrl+R` opens **history search**; `Ctrl+T` searches your aliases.

Inside any picker: type to filter, arrows to move, `Enter` to select,
`Esc`/`Ctrl+C` to cancel. In the inline command picker, `Tab` completes the
highlighted command into the input line.

## Your own pickers

`rune.ui.picker.show` gives scripts the same overlay:

```lua
rune.ui.picker.show({
    title = "Where to?",
    items = {
        { text = "temple",  desc = "the safe room",   value = "temple" },
        { text = "smithy",  desc = "repairs",         value = "smithy" },
        { text = "harbor",  desc = "boats east",      value = "harbor" },
    },
    on_select = function(value)
        rune.send("walkto " .. value)
    end,
})
```

`on_select` receives the selected item's `value`, which defaults to its
`text`. Items can also be plain strings. Pass `mode = "inline"` for a
compact picker attached to the input line instead of a modal (the `title`
only shows in modal mode), and `match_description = true` to fuzzy-match
descriptions too.

Bind it to a key and you have a custom menu:

```lua
rune.bind("f3", function()
    rune.ui.picker.show({ title = "Paths", items = path_names, on_select = go })
end)
```

**Related:** [Input & History](/interface/input/) for tab completion
and history navigation, [Key Bindings](/scripting/keybindings/)
