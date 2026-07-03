---
title: Groups
description: Master switches over sets of aliases, triggers, timers, hooks, binds, bars, and commands.
---

Anything you register (aliases, triggers, timers, hooks, binds, bars,
slash commands) can carry a `group`. A group is a master switch: an item
fires only if it is enabled and its group is enabled.

```lua
rune.trigger.contains("attacks you", "flee", { group = "coward" })
rune.alias.exact("cower", "hide",           { group = "coward" })
rune.timer.every(5, "peek",                 { group = "coward" })
```

```
/group coward off      -- everything above goes quiet
/group coward on       -- and comes back, individual states preserved
/groups                -- list groups and their state
```

From Lua: `rune.group.enable/disable/is_enabled(name)`, and
`rune.group.list()` returns an array of `{name, enabled}`.

## Patterns

**Mode switches.** A combat mode that arms spell-up triggers, or a quiet
mode that gags channels. Bind them to keys:

```lua
rune.bind("f2", function()
    if rune.group.is_enabled("spellup") then
        rune.group.disable("spellup")
    else
        rune.group.enable("spellup")
    end
    rune.ui.refresh_bars()
end)
```

**Bulk removal.** The alias, trigger, timer, hooks, binds, and bars
registries can each drop a whole group with
`rune.trigger.remove_group("quest-xyz")` and the equivalents. Useful when
a script builds temporary registrations.

## Gotchas

- Group toggles don't touch individual enabled/disabled states: disable
  one trigger inside an enabled group and it stays disabled when the
  group cycles.
- A disabled bind swallows its key rather than letting it through to
  typing, so disabling a group of binds mutes those keys entirely.

**Related:** [Aliases](/rune/scripting/aliases/),
[Triggers](/rune/scripting/triggers/),
[Timers](/rune/scripting/timers/)
