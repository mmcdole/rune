---
title: Storage & Worlds
description: Three lifetimes of state (plain variables, the session store, and the durable store) plus named server bookmarks.
---

Rune gives state three lifetimes. Pick by how long it should live:

| | Lives until | Values |
|---|---|---|
| Lua variables | `/reload` | anything |
| `rune.session` | client exit | strings |
| `rune.store` | forever (`store.json` on disk) | strings, numbers, booleans, tables |

## Session store

Survives `/reload` (the Lua VM is rebuilt) but not exit. Use it for
mid-session state you don't want a reload to wipe:

```lua
rune.session.set("kills", tostring(kills))
kills = tonumber(rune.session.get("kills") or "0")
```

Values are strings only; encode anything richer yourself.
`rune.session.delete(key)` removes one.

## Durable store

Backed by `~/.config/rune/store.json`, which is pretty-printed,
hand-editable, and written atomically on every `set`. Takes structured
values:

```lua
rune.store.set("prefs", { autoloot = true, greet = "Hail, %s!" })
local prefs = rune.store.get("prefs") or {}
```

`rune.store.set(key, nil)` (or `rune.store.delete(key)`) removes a key.

## Worlds

Named bookmarks, stored durably under the `"worlds"` key:

```txt
/world add viking vikingmud.org 2001
/world add secure mud.example.com 4000 tls
/worlds
```

Then `/connect viking`, `rune viking` from your shell, or bare `/connect`
for a picker. From Lua:

```lua
rune.world.add(name, address, opts?)  -- extra opts keys stored verbatim
rune.world.get(name)                  -- { address = ... }
rune.world.list()
rune.world.remove(name)
```

The `opts` table is yours: the [auto-login recipe](/cookbook/autologin/)
stores a character name per world and reads it back on connect.

## Gotchas

- Unstorable values (functions, cycles, mixed-key tables) return
  `nil, err` instead of writing — check the return when storing anything
  built at runtime.
- A corrupt `store.json` is preserved as `store.json.bak` at boot and
  reported, never silently discarded.
- `store.json` is plaintext on disk, so keep passwords out of it. The
  [auto-login recipe](/cookbook/autologin/) shows alternatives.

**Related:** [Storage reference](/reference/api/storage/),
[Slash command reference](/reference/slash-commands/)
for the full `/world` and `/connect` forms
