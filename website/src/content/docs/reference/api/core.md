---
title: Core
description: Sending, echoing, connecting, loading scripts, and quitting — plus the client data fields.
---

The top-level functions on the `rune` table. For a task-oriented
introduction, see [Scripting Basics](/getting-started/scripting-basics/).

## Quick reference

```lua
rune.send(text)        -- process aliases and expansion, then send
rune.send_raw(text)    -- straight to the socket, no processing
rune.echo(text)        -- print to the local display only
rune.connect(address)  -- "host:port", optional tls:// scheme
rune.disconnect()      -- close the connection
rune.load(path)        -- run a Lua script; true, or nil + error
rune.reload()          -- tear down the VM, re-run core + user scripts
rune.quit()            -- exit the client

rune.config_dir        -- path to the config directory (data, not a function)
rune.version           -- client version string
rune.debug             -- set true to enable rune.dbg output
rune.dbg(msg)          -- print msg, but only while rune.debug is true
```

`rune.echo` prints locally and never touches the server — pair it with
[rune.style](/reference/api/style/) for colored messages. `rune.reload`
tears down the Lua VM and re-runs the core plus your scripts;
[`rune.session`](/reference/api/storage/) state survives it.

### rune.send

```lua
rune.send(text)
```

- `text` (string) — input to process exactly as if you had typed it.

The full input pipeline: `;` splits the text into separate commands,
`#N` repeats expand, and each command runs through
[aliases](/reference/api/alias/) before going to the server. Repeats
are anchored at command position — `#3 north` repeats, but
`say #3 cheers` is chat text and passes through untouched. Alias
expansions are processed recursively (nested aliases work), with a
depth limit to catch loops.

```lua
rune.send("#2 {get bread bag;eat bread}")  -- get/eat, twice
```

### rune.send_raw

```lua
rune.send_raw(text) -> true | nil, err
```

- `text` (string) — sent to the server as-is: no aliases, no `;`
  splitting, no `#N` repeats.

Returns `true`, or `nil` plus an error message (which is also echoed)
when the send fails — typically because you're disconnected. This is
what alias and trigger string actions ultimately call.

### rune.connect

```lua
rune.connect(address)
```

- `address` (string) — `host:port` with an optional scheme prefix:

| Form | Connection |
|---|---|
| `host:port` | Plain telnet (default) |
| `tls://host:port` | TLS, certificate verified |
| `tls+insecure://host:port` | TLS, no verification (self-signed certs) |

The full address, scheme included, is what
[`rune.state.address`](/reference/api/state-lines/) reports and what
the core stores for `/reconnect`. Connecting is asynchronous — the
`"connecting"` and `"connected"` [hook events](/reference/api/hooks/)
report progress.

```lua
rune.connect("tls://mud.example.com:4000")
```

### rune.load

```lua
rune.load(path) -> true | nil, err
```

- `path` (string) — path to a Lua script; `~` expands to your home
  directory.

Runs the script immediately and returns `true`, or `nil` plus an error
message. While the script runs, its directory temporarily joins
`package.path`, so it can `require()` files relative to its own
location:

```txt
~/.config/rune/
├── init.lua              -- main script
├── combat.lua            -- require("combat")
└── utils/
    └── helpers.lua       -- require("utils.helpers")
```

```lua
-- In init.lua:
local combat = require("combat")         -- loads combat.lua
local helpers = require("utils.helpers") -- loads utils/helpers.lua
```

Standard Lua `require()` semantics apply: modules are cached after the
first load, and should return a table of exports.

## Data fields

`rune.config_dir` and `rune.version` are plain strings set by the
client. `rune.debug` is yours to flip — while it's `true`, `rune.dbg`
prints its message with a `[dbg]` prefix; otherwise it's silent, so
you can leave debug calls in place:

```lua
rune.debug = true
rune.dbg("trigger fired for " .. name)
```

**Related:** [Scripting Basics](/getting-started/scripting-basics/) ·
[State & Lines](/reference/api/state-lines/) ·
[rune.alias](/reference/api/alias/) · [Storage](/reference/api/storage/)
