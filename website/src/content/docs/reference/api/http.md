---
title: rune.http
description: Full signatures for async HTTP requests — webhooks, REST APIs, and fetching data into scripts.
---

Asynchronous HTTP requests. The request runs off the client's event
loop and your callback runs back on it, so a slow server never blocks
the UI. For a worked example, see the
[Telegram recipe](/cookbook/telegram/).

## Quick reference

```lua
rune.http.get(url, opts?, callback?)         -- GET
rune.http.post(url, body, opts?, callback?)  -- POST; body sent as-is
```

Both accept the same `opts` and `callback`; both may be omitted
(`rune.http.get(url, callback)` works). Without a callback the request
is fire-and-forget.

### rune.http.get

```lua
rune.http.get(url, opts?, callback?)
```

- `url` (string) — `http://` or `https://` only.
- `opts` (table, optional) — `headers` (name → value table) and
  `timeout` (seconds, default 30).
- `callback` (function, optional) — `function(response, err)`. Exactly
  one of the two is set.

```lua
rune.http.get("https://api.example.com/who", function(resp, err)
    if err then
        rune.echo("[who] " .. err)
        return
    end
    rune.echo("[who] " .. resp.body)
end)
```

### rune.http.post

```lua
rune.http.post(url, body, opts?, callback?)
```

- `body` (string) — sent exactly as given. No `Content-Type` is set by
  default; pass one in `opts.headers` when the server needs it
  (`application/x-www-form-urlencoded`, `application/json`, ...).

```lua
rune.http.post("https://example.com/hook", '{"event":"levelup"}',
    { headers = { ["Content-Type"] = "application/json" } })
```

## The response

| Field | Description |
|---|---|
| `response.status` | HTTP status code (number) |
| `response.body` | Response body (string) |
| `response.headers` | Response headers, name → first value |

`err` is set only when the request itself failed — DNS, timeout, TLS,
unsupported scheme, or a response body over the 5 MB cap. A non-2xx
status is a **response**, not an error: check `response.status`
yourself.

## Behavior

- The callback runs on the client's event loop under the script
  watchdog, like every other callback; a callback that throws is
  reported through the standard error path.
- `/reload` drops pending callbacks (they live in the Lua VM). A
  request still in flight completes, and its late result is silently
  discarded.
- Redirects are followed (up to Go's default of 10).

**Related:** [Telegram recipe](/cookbook/telegram/) ·
[Core](/reference/api/core/) ·
[rune.timer](/reference/api/timer/)
