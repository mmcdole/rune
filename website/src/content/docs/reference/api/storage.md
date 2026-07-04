---
title: Storage
description: Full signatures for the session store, the durable store, and world bookmarks.
---

Two Go-owned stores with different lifetimes, plus world bookmarks
built on the durable one. For a task-oriented introduction, see
[Storage & Worlds](/scripting/storage/).

## Quick reference

```lua
rune.session.set(key, value)  -- store a string (survives /reload)
rune.session.get(key)         -- the stored string, or nil
rune.session.delete(key)      -- remove a key

rune.store.set(key, value)    -- store durably; true or nil + err
rune.store.get(key)           -- the decoded value, or nil
rune.store.delete(key)        -- remove a key

rune.world.add(name, address, opts?)  -- save a bookmark; true or nil + err
rune.world.remove(name)               -- true if it existed
rune.world.get(name)                  -- entry table ({address=...}), or nil
rune.world.list()                     -- sorted array of {name, address}
```

The name encodes the lifetime:

| | `rune.session` | `rune.store` |
|---|---|---|
| Survives `/reload` | yes | yes |
| Survives client exit | **no** | **yes** (disk: `<config>/store.json`) |
| Values | strings only | strings, numbers, booleans, JSON-able tables |
| Use for | combat toggles, counters, mid-session scratch | bookmarks, settings, anything durable |

## rune.session

A string store scoped to this client session: it survives `/reload`
(the Lua VM is torn down and rebuilt) but not exit. Values are
strings; encode anything richer yourself. `get` returns `nil` for
unset keys.

## rune.store

### rune.store.set

```lua
rune.store.set(key, value) -> true | nil, err
```

- `key` (string) — the storage key.
- `value` — a string, number, boolean, or a JSON-able table (all
  string keys, or array shape `1..n`). `nil` deletes the key.

Returns `true`, or `nil` plus an error for unstorable values —
functions, userdata, mixed-key tables, cycles — in which case nothing
is written.

```lua
rune.store.set("kill_counts", { rat = 42, goblin = 7 })
local counts = rune.store.get("kill_counts")
```

`get` returns the decoded value, or `nil` if unset. `delete` removes
the key.

The store is backed by `store.json` under `rune.config_dir` —
pretty-printed, so it is hand-editable while the client is closed.
Writes hit disk immediately and atomically; reads are served from
memory. A corrupt file at boot is preserved as `store.json.bak` and
reported, never silently discarded.

## rune.world

Named server bookmarks, kept in `rune.store` under the `"worlds"`
key. `/connect <name>` resolves bookmarks before host:port parsing,
and bare `/connect` opens a picker over them.

### rune.world.add

```lua
rune.world.add(name, address, opts?) -> true | nil, err
```

- `name` (string) — the bookmark name; cannot contain spaces, `:`
  or `/`.
- `address` (string) — `host:port`, optionally with a `tls://` or
  `tls+insecure://` scheme.
- `opts` (table, optional) — extra keys stored verbatim alongside the
  address.

Adding an existing name replaces it. `remove(name)` returns `true` if
the bookmark existed; `get(name)` returns the stored entry table
(`{address = ...}`) or `nil`; `list()` returns a sorted array of
`{name, address}`. `/world add|remove|list` and `/worlds` drive the
same functions from the input line.

**Related:** [Storage & Worlds guide](/scripting/storage/) ·
[Core](/reference/api/core/) ·
[Slash Commands](/reference/slash-commands/)
