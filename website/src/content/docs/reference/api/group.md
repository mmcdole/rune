---
title: rune.group
description: Full signatures for group master switches — batch enable/disable across every registry.
---

Groups batch-toggle related items — aliases, triggers, timers, hooks,
binds, bars, and commands — with one master switch. For a
task-oriented introduction, see [Groups](/scripting/groups/).

## Quick reference

```lua
rune.group.enable(name)       -- master switch on
rune.group.disable(name)      -- master switch off
rune.group.is_enabled(name)   -- true/false; unknown groups default to true
rune.group.list()             -- array of {name, enabled} across all registries
```

## The two-level model

Every item has two independent enable switches: its own state
(`h:disable()`) and its group's master switch. An item fires only when
**both** are enabled. Disabling a group doesn't mutate individual
states — re-enabling the group restores each item exactly as it was.

```lua
rune.alias.exact("buff", "cast buff", {group = "combat"})
rune.trigger.starts("Enemy", "attack", {group = "combat"})
rune.timer.every(30, "heal", {group = "combat"})

rune.group.disable("combat")  -- all three stop firing
rune.group.enable("combat")   -- all three resume, individual states preserved
```

Groups don't need to be declared — naming one in an item's `group`
option (or toggling it) brings it into existence, and `is_enabled`
returns `true` for any group never disabled.

## Removing by group

`rune.group` only controls the master switches. Removal is
per-registry — `rune.trigger.remove_group("combat")`,
`rune.alias.remove_group("combat")`, and so on; see
[Registries](/reference/api/#managing).

## Slash commands

`/group <name> on|off` toggles a group mid-game without typing Lua;
`/groups` lists every known group and its state.

**Related:** [Groups guide](/scripting/groups/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.alias](/reference/api/alias/) ·
[rune.timer](/reference/api/timer/)
