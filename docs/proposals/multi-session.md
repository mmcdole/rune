# Multi-session support: config isolation, worlds, and characters

Status: proposal (revised after design review and MUD-client survey)

## Terminology

- **World** — the place: one MUD, one address (`aardwolf`). A named bookmark in the registry. Matches MUD vernacular and the existing `/world` command.
- **Character** — an optional label the user attaches at connect (`rune aardwolf:tank`). Rune never validates it against the game; it is the user's word for "who I'm playing" — a character, a class, `pk-alt`, whatever. Config may optionally hang off the name.
- **Connection** — the live link and its resolved identity: `{world, character, address}`. Runtime-only; never appears on disk.

## Problem

Rune is one process = one connection; playing two MUDs or two characters means two
processes, however the user arranges them (terminal windows, tmux, screen). That
workflow is under-supported:

1. **No config isolation.** No config-dir flag; the only override is setting
   `XDG_CONFIG_HOME` at launch, which is awkward and redirects anything else that
   process reads through XDG.
2. **Concurrent processes clobber shared state.** `store.json` is read once at boot and
   rewritten whole on every `set`, so two processes silently erase each other's writes —
   including the world bookmarks, which live inside it. Logs default to one shared file
   naming scheme.
3. **Scripts can't tell sessions apart.** Nothing exposes what world/character/address a
   session is connected to, so per-world behavior can't be scripted even by convention,
   and per-character state (kill counts, quest timers) collides across characters by
   construction.

The sharpest case: two characters on one MUD (e.g. Aardwolf), played simultaneously —
separate state and login identity, shared game-level config.

## Design principles

These fell out of the review and the client survey; every mechanism below follows them.

1. **One process = one connection.** Window arrangement is the user's business —
   terminal windows, tmux, screen all work. (Unchanged; see non-goals.)
2. **Flat containers, identity-prefixed names.** The same pattern answers "where does X
   live?" everywhere: `logs/aardwolf-tank-<ts>.log`, `worlds/aardwolf/tank.lua`, store
   key `aardwolf:tank/kills`. One idea, three surfaces (filenames use `-` only because
   `:` is illegal on Windows filesystems).
3. **Each kind of data has exactly one home.** Bookmarks → `worlds.json`. Script data →
   `store.json`. Config → `init.lua` + `worlds/`. Logs → `logs/`. No per-world stores,
   no per-character log dirs, no metadata files inside config dirs.
4. **One resolution path per layer.** "Optional" means a file may not exist — never that
   it may exist in several places. The single sanctioned duality is Lua's own
   `require`-style rule (`X.lua`, else `X/init.lua`), applied uniformly.
5. **Nothing is scaffolded.** Files and directories exist because the user created them,
   which is also how the user knows what they mean. `/world add` writes one JSON entry.
6. **Nothing is scoped unless you can see the word that scoped it.** Bare `rune.store`
   is global, always. Scoped state is `rune.store.world` / `rune.store.char` — scoped
   because it says so, at the write site. No call-time scope resolution, no silent
   fallbacks.

Survey grounding: no surveyed client (Mudlet, MUSHclient, zMUD/CMUD, TinTin++,
TinyFugue, Blightmud) ships a scope-fallback store; the proven models are hard container
isolation (Mudlet profiles — where cross-character *sharing* is the notorious weak spot)
and one global store with author-named keys (Blightmud). This proposal is the second
model with its two known gaps closed: identity exposure and ergonomic key scoping, plus
multi-process write safety that neither camp has.

## Mechanism 1 (coarse): `--config <dir>`

`rune --config ~/muds/tank` uses that directory as the complete home: its `init.lua`,
its `store.json`, its `worlds.json`, its `logs/`. `RUNE_CONFIG_DIR` is the env twin;
the flag wins. A directory containing only an `init.lua` is a complete rune experience.

This is whole-profile isolation for users who want fully separate homes, accepting
config duplication as the price. It is also the only mechanism a user ever *needs* —
everything below is optional refinement within one home.

With `--config` in place, **positional `.lua` script arguments are removed** (breaking
change). The CLI narrows to flags plus an optional connect target: `rune [--config
<dir>] [world[:char] | host:port]`. Every script rune runs is now reachable from the
config dir — global `init.lua` (which can `rune._load` anything else) and the `worlds/`
layers — so there is exactly one loading story, one boot path, and ad-hoc experiments
use a scratch `--config` dir instead of a side channel.

## Mechanism 2 (fine): worlds and characters as convention files

### Registry: `worlds.json`

World bookmarks move out of `store.json` into their own file:

```json
{
  "aardwolf": { "address": "aardmud.org:23" },
  "arctic":   { "address": "mud.arcticmud.org:2700" }
}
```

- `/world add`, `/world remove`, `/world list` operate on this file and nothing else.
- Entries may grow future fields (tls variants already ride in the address string).
- Readable and hand-editable without executing Lua.

**Names** (worlds and characters) are tightened to `[a-z0-9_-]+`. They become filename
prefixes and layer names; lowercase-only avoids case-insensitive-filesystem collisions
(macOS), and excluding `.` and `:` keeps both the layer-file convention and the
`world:character` target syntax unambiguous.

### Config layers: `worlds/`

A layer named `X` in directory `D` resolves exactly like Lua's `require`:
**`D/X.lua`, else `D/X/init.lua`.** If both exist, the file wins and rune warns about
the shadowed directory. Missing layers are silently skipped.

- World layer: resolve `<world>` in `worlds/`.
- Character layer: resolve `<character>` in `worlds/<world>/` — so character config
  implies the world is in directory form, the same growth move every Lua programmer
  knows (`foo.lua` becomes `foo/init.lua` when it needs siblings).

The natural progression:

```
worlds/aardwolf.lua                  # casual: one world, a bit of config

worlds/aardwolf/init.lua             # world outgrew one file
worlds/aardwolf/mapper.lua           #   loaded by init.lua however the user likes

worlds/aardwolf/init.lua             # multi-character
worlds/aardwolf/tank.lua

worlds/aardwolf/tank/init.lua        # character outgrew one file
worlds/aardwolf/tank/pk-triggers.lua
```

The directory is a **scripts home, nothing more**: no registry metadata inside it, no
per-directory store or logs. Deleting `worlds/aardwolf/` deletes config — bookmarks and
data live in their own single homes.

### Connect targets and identity resolution

`rune aardwolf`, `rune aardwolf:tank`; identical forms in `/connect`. Resolution:

1. Explicit `world:character` target → full identity.
2. Explicit `world` target → world identity, character nil.
3. Ad-hoc address uniquely matching one world's address → adopt that world's identity.
   On a genuine tie, connect anonymous and say so — never guess.
4. Unknown address → anonymous: world and character nil; `address` always present.

There is no `default_character`: identity is always what the user typed (or nothing).
A bare `/connect` opens the picker over `worlds.json`.

`/world add <name>` with no address adopts the current connection's address
(promote-after-exploring).

### Load order and identity switching

Boot: core → global `init.lua`. (CLI `.lua` script args no longer exist.)

On successful connect with identity: world layer → character layer → `connected` hook.
Layers therefore always run with `rune.connection` populated, and `connected` handlers
see a fully-configured session. Missing layer files are skipped silently.

Reconnecting to the same identity re-fires the `connected` hook but does **not** re-run
layer files — the VM already has them. The discipline this teaches: top-level layer
code is configuration (runs once per VM); per-connection actions (login sequences,
GMCP subscriptions) belong in a `connected` handler that the layer registers.

`/reload` while connected: fresh VM, then the full layered sequence — core → global →
world layer → character layer → `reloaded` hook. Identity survives reload on
the Go session actor, so world/character config is never silently stripped mid-play.
The connection stays up; no reconnect occurs and `connected` does not re-fire.

Connecting to a **different** world or character mid-session performs the equivalent of
`/reload` under the new identity — fresh VM, then the layered load, then connect. No
additive loading with manual unload; Lua globals cannot be un-polluted. Reconnecting to
the **same** identity does not reload.

Implementation note: `/reload` defers `boot()` onto the event queue and the CLI connect
target is consumed on first boot only — the switch path needs a pending-connect-target
that survives one reload cycle.

## Runtime API

```lua
rune.store.set(k, v)          -- global, persists to disk
rune.store.world.set(k, v)    -- per-world, persists      (key stamped <world>/)
rune.store.char.set(k, v)     -- per-char, persists       (key stamped <world>:<char>/)
rune.temp.set(k, v)           -- ephemeral, flat, no scope
rune.connection.world         -- read-only identity
rune.connection.character     -- read-only identity
rune.connection.address       -- read-only identity
```

| Name | What | Lifetime |
|---|---|---|
| `rune.store` | shared savefile (JSON-able values) | disk, forever |
| `rune.store.world` | same API, keys stamped `<world>/` | disk, forever |
| `rune.store.char` | same API, keys stamped `<world>:<char>/` | disk, forever |
| `rune.temp` | key-value that survives `/reload` | until quit |
| `rune.connection` | `.world` `.character` `.address`, read-only | until next connect |

Three names, three lifetimes: `store` is forever, `temp` is this run of rune,
`connection` is this connection.

- **`rune.session` is renamed `rune.temp`** (same semantics: survives `/reload`, gone at
  client exit). "Session" is the vocabulary for a live connection instance — a memory
  bag wearing that name misleads, especially one word away from `rune.connection`. The
  old name is **retired, not repurposed**: a boot-time tombstone error ("`rune.session`
  was renamed `rune.temp`") beats a name that silently changes meaning. Polish while
  renaming: `rune.temp` values widen from strings to the same JSON-able types as
  `rune.store`, so the API shapes match.
- **`rune.temp` is flat** — no `.world`/`.char` scope accessors until real demand shows
  up. Its contents die with the process, so cross-character collision is a much smaller
  surface; scripts that need scoped ephemera can key by `rune.connection` fields.
- **`rune.connection`** persists through disconnect until the next connect or reload, so
  `disconnected` handlers can read it ("the current or most recent connection"). It
  lives on the Go session actor so it survives `/reload`.
- **Store scope accessors** stamp keys with a **documented, transparent** prefix that
  mirrors the connect-target syntax: `store.world.set("map_notes", n)` on aardwolf
  writes the literal key `aardwolf/map_notes`; `store.char.set("kills", 5)` as
  `aardwolf:tank` writes `aardwolf:tank/kills` — inspectable and greppable in
  `store.json`. The two scopes
  carry distinct meanings: `store.world` is shared across your characters on that world
  (map data, area notes); `store.char` is this character only (kill counts, quest
  timers). Missing identity is an error — `store.world` without a world, or
  `store.char` without a character, returns `nil, err`; never a silent fallback to
  global.

## Store concurrency

The clobbering bug is independent of scoping and is fixed at the write path. Every
write to `store.json` (and `worlds.json`):

1. Take an exclusive file lock (`flock`).
2. **Re-read the file** — disk, not memory, is the source of truth.
3. Apply **only the current operation** (set/delete of one key) on top of what was read.
4. Write temp file + atomic rename; release the lock. The in-memory cache is replaced
   by what was written.

The contract, in three lines:

- Writes never lose another process's keys (delta-under-lock).
- Same key, two writers → last write wins. `store.char` makes cross-character
  collisions impossible by construction; on genuinely shared keys (`store.world`,
  bare `store`), last-wins is usually correct (`last_address`: the most recent connect
  should win `/reconnect`).
- Reads may be stale until your next write. The store is a savefile, not a message bus;
  cross-session communication is a different feature and must not be smuggled through it.

Existing corruption handling (rename to `.bak`, boot empty, report) is retained.

## Logs

One flat `logs/` directory; identity in the filename, most-specific parts dropped when
absent:

```
logs/aardwolf-tank-2026-07-17_20-15-03.log    # world + character
logs/aardwolf-2026-07-17_20-16-11.log         # world only
logs/aardmud.org_23-2026-07-17_20-18-40.log   # anonymous, connected (sanitized host)
logs/2026-07-17_20-18-40.log                  # not connected (today's behavior)
```

"Where are my logs?" has a one-word answer; `ls logs/` is the whole history, sorted;
concurrent sessions can never share a file. `rune.log.start(path)` is untouched. Pure
Lua change (`60_log.lua`).

## Migration

One-time at boot: entries under `store.json`'s `"worlds"` key move to `worlds.json`,
then the key is removed. Existing bookmarks are address-level, i.e. already worlds —
names carry over; names failing the tightened charset are sanitized (lowercased,
invalid characters replaced) with a boot notice. `last_address` stays in the store —
`/reconnect` then re-adopts full identity via address matching for free. User store
data is untouched.

Two breaking changes, each with a clear error message:

- `rune.session` → `rune.temp`, with a boot-time tombstone error for old callers
  naming the new location.
- Positional `.lua` CLI args are removed; passing one produces an error pointing at
  `init.lua` / `--config`. Users move the script into their config dir (or `rune._load`
  it from `init.lua`).

## Go/Lua split

Go (primitives only):

- `--config` flag / `RUNE_CONFIG_DIR` wiring in `cmd/rune` (`session.Config.ConfigDir`
  already exists), and removal of positional `.lua` script args
  (`session.Config.UserScripts` and its loader path go away).
- The locked delta-write path in the store primitive (prefix-agnostic; `store.world`
  and `store.char` are pure Lua sugar).
- Connection identity held on the session actor across reload, with a set-primitive Lua
  calls after resolution and a read-only view exposed as `rune.connection`.
- Pending-connect-target support in the reload machinery.

Everything else — target parsing, identity resolution, `worlds.json` handling, layer
resolution and loading (via the existing `rune._load`), migration, log paths,
the store scope accessors, `/world` UX — is Lua core.

## Testing

Per `docs/testing.md`, lowest layer that can express each failure:

- Store delta-write concurrency: Go unit test, two writers interleaving set/delete.
- Target parsing, identity resolution, layer resolution (file-vs-dir, shadow warning),
  `store.world`/`store.char` key stamping and nil-identity errors: Lua layer against
  MockHost.
- Connect flow, layer load order relative to hooks, reload-on-switch with pending
  target: session-synchronous tests.
- One e2e scenario: "connect to `world:char` loads both layers, `connected` sees
  `rune.connection`."

## Delivery phases

Each lands independently useful:

1. **`--config` + `RUNE_CONFIG_DIR`, and removal of positional `.lua` args.** The CLI
   becomes `rune [--config <dir>] [target]`; whole-profile isolation ships immediately.
2. **Locked delta-writes + `worlds.json` split + migration.** Concurrent processes stop
   eating each other's data — including everyone already running two rune windows
   today.
3. **`rune.connection` + connect targets + address adoption + store scopes + the
   `rune.temp` rename.** Scripts can tell sessions apart (the ecosystem's actual
   most-requested capability) and partition state in one word; all API-surface changes
   land together.
4. **`worlds/` layers + reload-on-switch + log filenames.** Convenience on top: per-world
   and per-character config by convention.

## Non-goals

- **In-process multi-session (tabs).** The user's terminal environment already provides
  multiplexing, attention, and fault isolation. If ever built: N (VM) pairs over an explicit inter-session event channel —
  for which identity and the concurrency-safe store are prerequisites; this proposal
  deliberately builds those first.
- **Credentials/auto-login.** The character layer and `worlds.json` entries are the
  obvious future homes; storage policy deserves its own issue.
- **Auto-migrating user store data into prefixed keys.** Existing keys mix per-character
  and shared data we cannot classify; they stay global and keep working.
- **Cross-session messaging.** The store's staleness contract explicitly excludes it;
  an explicit channel (Mudlet's `raiseGlobalEvent` shape) is future work if demanded.

## Decisions log (from review)

1. Registry is a file (`worlds.json`), not directories and not Lua-declared — readable
   without executing Lua, trivially written by `/world add`, conflict-free under the
   locked write path.
2. Bare `rune.store` stays global forever; scoping is explicit (`store.world`,
   `store.char`). Survey: no client ships call-time scope fallback; CMUD's automatic
   session scoping is safe only because its code never runs outside a session — rune's
   global scripts do. Two named scopes beat a single "my" accessor because they express
   different things: world scope is shared across your characters (map data), char
   scope is this character only.
3. No `/char` command; a character exists by being named at connect or by its layer file
   existing. `/world list` may show discovered character layers.
4. No `default_character` — identity is always explicit.
5. `rune.session` → `rune.temp`: "session" is connection vocabulary and a memory bag
   wearing it misleads, one word away from `rune.connection`. The name is retired with
   a tombstone error, never repurposed — identity is `rune.connection` (rhymes with
   `/connect` and the connect hooks). Three names, three lifetimes: `store` forever,
   `temp` this run, `connection` this connection. `temp` stays flat (no scope
   accessors) until real demand appears.
6. Names: `[a-z0-9_-]+`, lowercase.
7. Layer resolution is `require`-style (`X.lua` else `X/init.lua`), the single
   sanctioned file-or-directory duality; character layers resolve inside the world's
   directory form.
8. Positional `.lua` CLI args are removed. `--config` supersedes them as the coarse
   mechanism and `worlds/` layers as the fine one; everything rune loads is reachable
   from the config dir, so there is one loading story and one boot path.
