---
title: rune.gmcp
description: Full signatures for GMCP — package handlers, sending, subscriptions, and the Core.Hello handshake.
---

GMCP carries structured out-of-band data — vitals, room info, comm
channels — over telnet option 201. For a task-oriented introduction,
see [GMCP](/scripting/gmcp/).

## Quick reference

```lua
rune.gmcp.on(package, handler, opts?)   -- handle a package (case-insensitive)
rune.gmcp.send(package, value?)         -- send a JSON-able value
rune.gmcp.send_raw(package, raw_json)   -- send pre-encoded JSON (debugging)
rune.gmcp.subscribe(package, version?)  -- declare interest (Core.Supports.Set)
rune.gmcp.unsubscribe(package)          -- withdraw interest
rune.gmcp.is_enabled()                  -- true once GMCP is negotiated
rune.gmcp.list()                        -- all handlers, as /gmcp shows them
```

`rune.gmcp.on` returns a [handle](/reference/api/#handles) and accepts
the [common options](/reference/api/#options).

### rune.gmcp.on

```lua
rune.gmcp.on(package, handler, opts?) -> handle
```

- `package` (string) — the package to match, exactly (`"Char.Vitals"`
  does not catch `"Char.Vitals.Max"`). Matching is case-insensitive,
  per the spec.
- `handler` (function) — `function(data, package)`. `data` is the
  decoded JSON value — a table, string, number, or boolean, or `nil`
  when the message had no body. `package` is the name as the server
  sent it.
- `opts` (table, optional) — [common options](/reference/api/#options)
  (`name`, `group`, `priority`).

Handlers are registry items like triggers, so
[quarantine](/reference/api/#quarantine) and source attribution apply.

```lua
rune.gmcp.on("Char.Vitals", function(data)
    hp, maxhp = data.hp, data.maxhp
    rune.ui.refresh_bars()
end, { name = "vitals" })
```

For every message regardless of package, the `"gmcp"`
[hook event](/reference/api/hooks/) fires with
`(package, data, raw)` — raw JSON text included. Malformed JSON from
the server is reported and dropped before any handler runs.

### rune.gmcp.send

```lua
rune.gmcp.send(package, value?) -> true | nil, err
```

- `package` (string) — the message name (`"Char.Skills.Get"`).
- `value` (optional) — a string, number, boolean, or JSON-able table;
  `nil` sends the bare package name.

Returns `true`, or `nil` plus an error when the send cannot happen —
not connected, GMCP not negotiated, or the value cannot be encoded.
Failures are also echoed to the screen.

```lua
rune.gmcp.send("Char.Skills.Get", { group = "combat" })
rune.gmcp.send("Core.Ping")
```

`send_raw(package, raw_json)` skips encoding and validation entirely —
a debugging tool (`/gmcp send` uses it).

## Subscriptions and the handshake

`subscribe(package, version?)` declares interest in a server package
(`"Char"`, `"Room"`, …); `version` defaults to 1. Subscriptions
maintain `Core.Supports.Set` — the full set is re-sent on every
change, per the spec — taking effect immediately when GMCP is up,
otherwise at the next handshake.

When the server negotiates GMCP, the `"gmcp_enabled"` event fires and
the core handler named `gmcp-hello` sends
`Core.Hello {client, version}` plus the current subscription set.
Subscribe at load time; the handshake picks it up on connect. Disable
or replace `gmcp-hello` like any named hook.

```lua
rune.gmcp.subscribe("Char")
rune.gmcp.subscribe("Room", 2)
```

## Managing

`rune.gmcp.enable/disable/remove(name)` manage handlers by name;
`rune.gmcp.list()` returns them — see
[Registries](/reference/api/#managing). `/gmcp` shows negotiation
state, subscriptions, and handlers; `/gmcp send <package> [json]`
sends a raw message for debugging.

**Related:** [GMCP guide](/scripting/gmcp/) ·
[rune.hooks](/reference/api/hooks/) ·
[Protocols](/reference/protocols/)
