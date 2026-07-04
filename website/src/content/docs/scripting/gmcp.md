---
title: GMCP
description: Structured server data such as vitals, rooms, and channels, delivered as Lua events.
---

GMCP delivers out-of-band data as `Package.SubPackage {json}` messages.
Rune decodes the JSON for you: handlers receive real Lua tables.

```lua
rune.gmcp.subscribe("Char")

rune.gmcp.on("Char.Vitals", function(data)
    hp, maxhp = data.hp, data.maxhp
    rune.ui.refresh_bars()
end)
```

## Receiving

```lua
rune.gmcp.on(package, handler, opts?)   -- handler(data, package); case-insensitive match
rune.hooks.on("gmcp", function(package, data, raw) ... end)  -- catch-all
```

`data` is the decoded value (table, string, number, boolean, or `nil` for
bodyless messages like `Core.Ping`). `package` is the name as the server
sent it, and `raw` is the JSON text.

## Sending

```lua
rune.gmcp.send("Char.Skills.Get", { group = "combat" })  -- any JSON-able Lua value
rune.gmcp.send("Core.Ping")                              -- bare package
```

Returns `true` or `nil, err` (not connected, GMCP not negotiated).
`rune.gmcp.send_raw(package, json?)` skips encoding and sends the string
as-is. `rune.gmcp.is_enabled()` reports whether GMCP is negotiated on the
current connection.

## Subscriptions and the handshake

`rune.gmcp.subscribe("Char")` declares interest and `unsubscribe` retracts
it. Rune maintains `Core.Supports.Set` for you, re-sending the full set on
every change. When a server negotiates GMCP, the `gmcp_enabled` event fires
and a core handler (named `gmcp-hello`) sends `Core.Hello` plus your
subscriptions. Subscribe at load time and the handshake picks it up on
connect. Replace the handler if your server needs a different hello.

## Options

Handlers ride the same registry as everything else and take the
[common options](/scripting/model/#options) `name`, `group`, and
`priority` (order among handlers for the package).

## Managing

Constructors return a handle with `:enable()`, `:disable()`, and
`:remove()` — full signatures in the
[rune.gmcp reference](/reference/api/gmcp/). In the client, `/gmcp` lists
negotiation state, subscriptions, and every handler with its source.

## Debugging

```txt
/gmcp                          -- negotiation state, subscriptions, handlers
/gmcp send Char.Skills.Get {}  -- send raw JSON
```

Log everything while exploring what a server offers:

```lua
rune.hooks.on("gmcp", function(package, data, raw)
    rune.dbg(package .. " " .. tostring(raw))
end)
-- with rune.debug = true
```

## Gotchas

- Malformed JSON from the server is reported once and dropped; it never
  reaches handlers.
- A handler that errors three times in a row is
  [quarantined](/scripting/model/#quarantine).

**Related:** [rune.gmcp reference](/reference/api/gmcp/),
[HP bar from GMCP](/cookbook/hp-bar/),
[Protocols reference](/reference/protocols/)
