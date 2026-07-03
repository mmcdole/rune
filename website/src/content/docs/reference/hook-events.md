---
title: Hook Events
description: Every event rune.hooks.on can attach to.
---

## Data-flow events

Handlers run in priority order (lower first, default 50).

For `output`, `prompt`, and `echo`: `nil` passes through, a string
replaces the text for subsequent handlers (rewrites chain), and `false`
stops the chain (gag or hide).

For `input`: `false` consumes the line; other return values are ignored.
Input handlers cannot rewrite by returning a string. Use `rune.input.set`,
or call `rune.send` yourself and return `false`.

| Event | Handler receives | Fired |
|---|---|---|
| `input` | typed text | On every submitted input line |
| `output` | line object (`:raw()`, `:clean()`) | On every complete server line |
| `prompt` | line object | On prompt fragments (no newline, or GA/EOR terminated) |
| `echo` | typed text | On local echo of submitted input; skipped while the server has echo suppressed (passwords) |

The core registers its own handlers at priority 100: command dispatch
and `rune.send` on `input`, trigger processing on `output`/`prompt`,
the `> ` styling on `echo`. For `output`/`prompt`/`echo`, register
below 100 to run before the core, or above 100 to see its results
(post-trigger rewrites; gagged lines never reach you). The core
`input` handler always returns `false`, so `input` handlers must
register below 100 to run at all.

## Notification events

All handlers run; return values are ignored.

| Event | Args | Fired |
|---|---|---|
| `ready` | none | Boot complete, after user scripts load (fires again on `/reload`) |
| `connecting` | address | Dial started |
| `connected` | address | Connection established |
| `disconnecting` | none | Disconnect requested |
| `disconnected` | none | Connection closed |
| `reloading` / `reloaded` | none | Around `/reload` (order: `reloading`, `ready`, `reloaded`) |
| `loaded` | path | After `/load` or `rune.load` loads a file (not for startup auto-load) |
| `error` | message | On reported errors |
| `input_changed` | text | As the input line changes while typing |
| `gmcp` | package, data, raw JSON | On every GMCP message, before package-specific `rune.gmcp.on` handlers |
| `gmcp_enabled` | none | GMCP negotiated; the core handler sends `Core.Hello` |

## Named core handlers

Handlers the core registers under stable names, so you can disable or
replace them: `log-output`, `log-echo` (logging policy, priority 200),
`gmcp-hello` (the GMCP handshake), `gmcp-reset`, `first-run-welcome`,
and `_completion_cache` / `_completion_input` (tab-completion word
harvesting, priority 200).

**Related:** [Hooks & Events](/scripting/hooks/)
