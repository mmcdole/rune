# Enhancement options (pending decision)

Status: proposal only, nothing implemented. Each option lists effort and
a recommendation; pick freely.

## Logging

Context: `/log` compared against TinTin++, Mudlet, and MUSHclient
(2026-07). Current behavior that already matches or beats the field and
should not change: append mode, ANSI-stripped plain text by default,
post-trigger logging (TinTin++'s `log_level high` default does the same),
automatic password exclusion via echo suppression, survives `/reload`,
write-failure closes the log with a single report.

### L1. World-aware default filename — recommended

`default_path()` in `60_log.lua` currently yields
`logs/2026-07-04_13-52-01.log`; logs from different MUDs interleave
anonymously in the directory. Derive a prefix when connected:

```
logs/viking_2026-07-04_13-52-01.log            -- bookmark name when the
logs/mud.example.com_4000_2026-07-04_13-52-01.log  -- address matches a world
logs/session_2026-07-04_13-52-01.log           -- disconnected
```

Reverse-lookup `rune.world.list()` for the bookmark name; sanitize
`:` and `/` out of addresses. Keep the seconds component: it makes
concurrent sessions of the same world collide only within the same
second, and even then `LogWrite` is one `write()` per line on an
`O_APPEND` handle, so a collision interleaves lines rather than
corrupting them.

Effort: ~10 lines of Lua. No Go changes.

### L2. `/log start` switches files instead of refusing — recommended

The Go host (`LogStart`) already closes and replaces an open log; the
refusal lives only in the `/log` command layer. TinTin++ switches.
Echo `[Log] switched: old -> new`. Makes always-on logging cleaner when
reconnecting elsewhere.

Effort: ~6 lines of Lua.

### L3. Docs: always-on recipe + keep-the-colors example — recommended

- Cookbook recipe "Always-on session logs": a `connected` hook starting
  a per-world **daily** file (`viking_2026-07-04.log`) so append mode
  yields one file per world per day. Multi-session note: when a world
  stores a character name (the auto-login recipe's `opts` pattern),
  include it — `viking_ragnar_2026-07-04.log` — so multi-boxing the
  same world produces per-character transcripts instead of an
  interleaved shared file. Fall back to the per-session timestamp form
  otherwise.
- Logging guide section "Keep the colors": disable `log-output`,
  re-register writing `line:raw()`, view with `less -R`.

Effort: docs only.

### L4. Built-in raw (color) logging — optional

TinTin++ (`log_mode raw|html`) and Mudlet (HTML toggle) both ship
color-preserving logs; in rune it currently requires replacing a hook.
Add `rune.log.start(path?, opts?)` with `{raw = true}`: the module
keeps a mode flag, `log-output`/`log-echo` branch on it
(`line:raw()` vs `line:clean()`), `/log start raw [file]` from the
command line. Policy hooks remain replaceable exactly as before.

Alternative: skip the API and rely on L3's documented hook swap.

Effort: ~20 lines of Lua. No Go changes.

### L5. Log GA/EOR-terminated prompts — hold

Prompts are currently excluded because unterminated prompts repeat on
every flush. But GA/EOR-delimited prompts arrive exactly once and often
carry the HP line a combat log wants. Requires Go to expose whether a
prompt was explicitly terminated (e.g. `line:terminated()`), then the
log policy logs only those. Hold until a target server makes it worth
the new line-object surface.

Effort: small Go change (line object field + plumbing) + 3 lines of Lua.

### Non-goals

- HTML logs (balloon in size; raw ANSI + `less -R` covers it)
- Built-in per-line timestamps (4-line hook, documented in the guide)
- Rotation/size caps (daily naming from L3 gets there for free)
- Cross-process file locking for shared logs (naming solves it better)

## Native HTTP primitive

Context: the Telegram cookbook recipe shells out
(`os.execute("curl ... &")`) because the embedded VM has no HTTP at
all — gopher-lua is pure-Go Lua 5.1, and LuaSocket (the usual Lua HTTP
answer) is a C library it cannot load. The recipe is careful
(`sh_quote`, backgrounded, `--max-time`) but the pattern is inherently:
POSIX-only, dependent on curl existing, fire-and-forget (no way to read
the response), and it pushes shell-injection safety onto every user
script that touches game text.

### H1. `rune._http` async primitive — recommended if webhooks matter

Go-owned HTTP client behind a `rune._http.request` primitive, surfaced
as `rune.http.get/post(url, opts?, callback?)` in the Lua core.
Fits the existing architecture exactly: Go does the I/O off the session
goroutine and delivers the result through the same `AsyncResult` event
that reload/connect already use, so the callback runs on the session
goroutine under the watchdog like every other callback. No blocking, no
shell, no quoting, works on Windows, response body available.

Precedent: Mudlet ships `getHTTP`/`postHTTP`/`downloadFile`;
MUSHclient has HTTP via utils; TinTin++ shells out like rune does
today.

Scope guard: GET/POST with headers/body/timeout and a
`(response, err)` callback. Not a general networking stack — no
streaming, no websockets, redirect/size limits fixed.

Effort: ~150 lines of Go (primitive + AsyncResult plumbing + tests) +
~30 lines of Lua core + an API reference page (the coverage test will
demand it). The Telegram recipe then drops `sh_quote` entirely.

### H2. Status quo — fine if webhooks stay niche

Keep the shell-out pattern and its documented safety habits. The recipe
already teaches the right hygiene, and `os.execute` remains available
for non-HTTP integrations regardless.
