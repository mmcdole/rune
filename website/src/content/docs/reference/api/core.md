---
title: Core
description: Sending, echoing, connecting, loading scripts, and quitting — plus the client data fields.
---

The top-level functions on the `rune` table. For a task-oriented
introduction, see [Scripting Basics](/getting-started/scripting-basics/).

## Quick reference

```lua
rune.send(text)        -- process command syntax and aliases, then send
rune.send_raw(text)    -- game lines, bypassing command processing
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

rune.config.get(key)          -- read a typed configuration value
rune.config.set(key, value)   -- validate and update a configuration value
```

`rune.echo` prints locally and never touches the server — pair it with
[rune.style](/reference/api/style/) for colored messages. `rune.reload`
tears down the Lua VM and re-runs the core plus your scripts;
[`rune.session`](/reference/api/storage/) state survives it.

### rune.send

```lua
rune.send(text)
```

- `text` (string) — command text to process programmatically.

The full command pipeline: the configured delimiter (`;` by default) splits
the text into separate commands, `#N` repeats expand, and each command runs
through [aliases](/reference/api/alias/) before going to the server. Repeats
are anchored at command position — `#3 north` repeats, but
`say #3 cheers` is chat text and passes through untouched. Alias
expansions are processed recursively (nested aliases work), with a
depth limit to catch loops.

`rune.send` does not run interactive input hooks or add history. In particular,
`rune.send("!")` sends a literal `!`; shell-style history expansion happens
only for interactive command submissions.

```lua
rune.send("#2 {get bread bag;eat bread}")  -- get/eat, twice
```

### rune.send_raw

```lua
rune.send_raw(text) -> true | nil, err
```

- `text` (string) — sent as game lines: no aliases, no `;`
  splitting, and no `#N` repeats. Text containing newlines is split and
  sent one physical line at a time. LF, CRLF, and bare CR are line breaks;
  empty lines are preserved.

Despite its name, `send_raw` sends MUD text, not Telnet or GMCP protocol
data. Returns `true`, or `nil` plus an error message (which is also echoed)
when the send fails — typically because you're disconnected. This is
what alias and trigger string actions ultimately call.

Before processing any user submission, Rune finishes an active partial line:
it commits the prompt overlay and closes open multiline spans before input
hooks and any local echo, history, aliases, or slash commands. This is
independent of local echo, connection state, whether a hook consumes the
submission, and whether it eventually sends anything.

A game line sent programmatically by an alias, trigger, timer, or other Lua
callback also finishes the partial line, once the connection accepts it for
writing. A failed programmatic send leaves the line open. GMCP and other
Telnet protocol traffic never finish it. If the partial line was really part
of an ordinary line, Rune commits the visible prefix and treats later bytes
as a new line.

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
client. `rune.config_dir` reflects `--config-dir`, `RUNE_CONFIG_DIR`, or
the platform default, in that order. Set `rune.debug` to `true` to make
`rune.dbg` print messages with a `[dbg]` prefix. When it is `false`,
`rune.dbg` does nothing:

```lua
rune.debug = true
rune.dbg("trigger fired for " .. name)
```

## rune.config

Go owns Rune's typed configuration, including its schema, validation, and
defaults. Read and update values through `get` and `set`; direct property
assignment is rejected.

```lua
rune.config.get(key)          -- value
rune.config.set(key, value)
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `delimiter` | string, non-empty | `";"` | Separator for chaining commands in one input line |
| `keep_input` | boolean | `false` | Keep an authored command in the input line, selected: `Enter` resends it, typing replaces it |

An unknown key, a value of the wrong type, or an empty `delimiter` raises an
error and leaves the configuration unchanged.

```lua
rune.config.set("keep_input", true)
rune.config.set("delimiter", "|")
assert(rune.config.get("keep_input") == true)
```

At normal runtime, a successful `set` takes effect immediately. Startup and
`/reload` are transactional: Rune starts a candidate at the defaults, evaluates
core scripts, user scripts, and ready hooks against it, then publishes one
complete snapshot. A consumer never observes the intermediate defaults or
staged updates. If a user script does not set a key again, the published value
reverts to its default.

**Related:** [Scripting Basics](/getting-started/scripting-basics/) ·
[State & Lines](/reference/api/state-lines/) ·
[rune.alias](/reference/api/alias/) · [Storage](/reference/api/storage/)
