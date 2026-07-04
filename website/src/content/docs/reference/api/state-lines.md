---
title: State & Lines
description: The read-only client state proxy, and the line object contract for output handlers.
---

Two small contracts the rest of the API leans on: `rune.state` exposes
live client state to renderers like [bars](/interface/bars/) and
[layout](/interface/layout/) code, and line objects are what
[hook](/scripting/hooks/) and trigger handlers receive for server
output.

## Quick reference

```lua
rune.state.connected     -- bool, connection status
rune.state.address       -- server address, scheme included
rune.state.scroll_mode   -- "live" or "scrolled"
rune.state.scroll_lines  -- new lines arrived while scrolled
rune.state.width         -- terminal width
rune.state.height        -- terminal height

rune.line.new(text)      -- build a line object from plain text
line:raw()               -- the line with ANSI codes intact
line:clean()             -- the line with ANSI codes stripped
```

## rune.state

A read-only proxy over live, client-owned state — reads always reflect
the current values, and writing any field raises an error.

| Field | Type | Description |
|---|---|---|
| `connected` | bool | Whether a connection is up |
| `address` | string | The connected address, scheme included (e.g. `tls://mud.example.com:4000`) |
| `scroll_mode` | string | `"live"`, or `"scrolled"` while scrolled back |
| `scroll_lines` | number | New lines received while scrolled |
| `width` | number | Terminal width in columns |
| `height` | number | Terminal height in rows |

Because it's always current, `rune.state` is the natural input for
[bar renderers](/interface/bars/):

```lua
rune.ui.bar("status", function(width)
    local s = rune.state
    local left = s.connected
        and rune.style.green("●") .. " " .. s.address
        or rune.style.gray("● Disconnected")
    return { left = left }
end)
```

## Line objects

Server output arrives in handlers as line objects, not plain strings:
`"output"` and `"prompt"` [hook](/reference/api/hooks/) handlers
receive one, and trigger function actions get one as
[`ctx.line`](/scripting/model/#the-context-object). Two methods:

```lua
line:raw()    -- ANSI codes included; use when re-emitting styled text
line:clean()  -- ANSI stripped; use when matching or parsing
```

`:clean()` is computed lazily and cached, so calling it repeatedly is
cheap.

### rune.line.new

```lua
rune.line.new(text) -> line
```

- `text` (string) — the raw text, ANSI codes and all.

Builds a line object compatible with what handlers receive. You rarely
need this — the main use is constructing a rewritten line to pass along,
or feeding synthetic lines through code that expects the line contract:

```lua
local l = rune.line.new("\027[31malert\027[0m")
rune.echo(l:clean())  -- "alert"
```

**Related:** [Hooks guide](/scripting/hooks/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.ui](/reference/api/ui/) · [Core](/reference/api/core/)
