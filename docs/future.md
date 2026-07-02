# Future: Protocols and Extensions

A roadmap for the MUD protocols and client features found in major
clients (Mudlet, TinTin++, blightmud, MUSHclient) that rune does not
have yet, in the rough order they should land. This is a planning
document: nothing here is committed work, and phases can be reordered
by demand.

> **Status:** Phases 1-3 have shipped - TTYPE/MTTS, NAWS, CHARSET,
> MNES (`network/negotiate.go`), MCCP2 (client.go read path), and GMCP
> (transport in Go, `rune.gmcp` policy in `59_gmcp.lua`). Their
> sections below are kept as design records. Next up: Phase 4.

## Ground Rules

Everything below is constrained by the existing architecture, and the
plan fails review if it violates one of these:

1. **Never advertise what isn't implemented.** The compatibility table
   (`network/telnet.go`, `defaultCompatibility()`) only `Support()`s
   options with real behavior behind them. Agreeing to TTYPE or NAWS
   and then answering garbage breaks real servers. Each protocol phase
   flips its option on only in the commit that implements it.
2. **Go is transport, Lua is policy.** Go parses, decompresses,
   frames, and negotiates; what to *do* with a GMCP message or an MSDP
   variable is Lua's decision, delivered through the existing hook
   system. Protocol data crosses the boundary through `rune._*`
   primitives and hook events, like everything else.
3. **Degraded mode stays intact.** A protocol failure (bad JSON from
   the server, zlib error) must degrade that protocol, not the client.
   The plain-telnet path must never depend on any of this.
4. **The parser stays a pure state machine.** `network/telnet.go`
   already emits `TelnetEventSubnegotiation` and
   `TelnetEventDecompressImmediate` and defines the option constants
   (`OptTTYPE`, `OptNAWS`, `OptMSSP`, `OptMCCP2/3`, `OptGMCP`, ...).
   New protocols consume those events in the connection layer; the
   parser only learns new framing, never semantics.

---

## Phase 1: Identity and Terminal Negotiation

Small, low-risk options that materially improve how servers treat the
client. Many MUDs key feature detection off these before anything else.

### TTYPE / MTTS (option 24)

Answer the standard terminal-type cycle per the MTTS spec:

1. First `SEND` → client name: `RUNE`
2. Second → terminal type: `XTERM` (or from `$TERM`)
3. Third+ → `MTTS <bitvector>`: ANSI + UTF-8 + 256 COLORS + TRUECOLOR
   (+ TLS bit once negotiated, + OSC color palette as UI support allows)

Pure Go: a responder in the connection layer keyed off
`TelnetEventSubnegotiation`. The MTTS bits should be computed, not
hardcoded, so they stay honest as UI capabilities change.

### NAWS (option 31)

Report window size. The session already tracks
`clientState.Width/Height` from `WindowSizeChangedMsg`; NAWS is a
subnegotiation containing those two values, re-sent on every resize
while the option is enabled. Needs one small plumbing addition:
Session tells the connection about size changes (today only Lua and
the UI know).

### CHARSET (option 42)

Accept the server's `REQUEST` and answer `UTF-8` when offered;
`REJECT` otherwise. The client is already UTF-8-native internally, so
this is negotiation-only.

### MNES / NEW-ENVIRON (option 39)

Answer `CLIENT_NAME`, `CLIENT_VERSION`, `CHARSET`, `MTTS`, and
`IPADDRESS`-style variables per the MNES spec. Overlaps with
TTYPE/MTTS; implement second and share the capability table.

**Testing:** all four are pure request/response — table-driven parser
tests with recorded byte sequences, same style as `telnet_test.go`.

---

## Phase 2: Compression - MCCP2 (86) / MCCP3 (87)

The single biggest quality-of-life protocol for players on large
MUDs. Entirely invisible to Lua.

- **MCCP2** (server→client): on `IAC SB 86 IAC SE`, the read path
  switches to streaming zlib inflate. The parser already emits
  `TelnetEventDecompressImmediate` with the remaining payload for
  exactly this handoff - the connection layer feeds those bytes plus
  the socket through a `zlib.Reader` from then on.
- **MCCP3** (client→server) is rare; implement only after MCCP2 has
  soaked, if at all.
- **Failure policy:** a zlib error is a hard connection error (the
  stream is unrecoverable) - disconnect with a clear message, per
  ground rule 3 the client itself stays healthy.

**Testing:** loopback server (the TLS test pattern in
`client_test.go`) that negotiates MCCP2 and streams a compressed
fixture; assert the decompressed lines come out of `Output()`.

---

## Phase 3: GMCP (option 201) - the flagship

Structured out-of-band data (vitals, room info, comm channels). This
is the protocol that makes bars, mappers, and chat panes real, and it
should be treated as rune's marquee scripting surface.

### Go side (transport only)

- Advertise GMCP; on `TelnetEventSubnegotiation` for option 201, split
  `Package.SubPackage <json>` into package string + raw JSON bytes and
  emit a new `network.Output` kind (`OutputGMCP`), which Session turns
  into an event.
- Sending: `rune._gmcp.send(package, value)` → `IAC SB 201 <package>
  <json> IAC SE` through the existing single `writeLoop`.
- **Reuse the JSON bridge.** `lua/api_store.go` already has
  `luaToGo`/`goToLua` with cycle/depth protection. GMCP encode/decode
  uses the same functions - one JSON↔Lua implementation in the repo.

### Lua side (policy, new core script)

```lua
rune.gmcp.send("Core.Hello", { client = "Rune", version = rune.version })
rune.gmcp.subscribe("Char")          -- maintains Core.Supports.Set
rune.hooks.on("gmcp", function(package, data) ... end)
-- plus per-package convenience: rune.gmcp.on("Char.Vitals", fn, opts?)
```

- Handshake policy (Core.Hello, Core.Supports.Set, keepalive) lives in
  the core script, driven from the `"connected"` event - visible,
  overridable, not buried in Go.
- `rune.gmcp.on` registrations go through `rune.registry.new`, so GMCP
  handlers get the same names, groups, quarantine, and source
  attribution as every other callback, and a `/gmcp` listing command
  falls out for free (plus `/gmcp send <pkg> <json>` for debugging).
- Malformed JSON from the server: report once via the `"error"` event,
  drop the message, keep the connection.

**Testing:** loopback negotiation test; JSON-driven feature tests for
dispatch (`gmcp_tests.json`); round-trip through the shared bridge.

---

## Phase 4: MSDP (option 69) and MSSP (option 70)

- **MSDP:** variable/value protocol with its own binary framing
  (`VAR`/`VAL`/`TABLE_OPEN`...). Go parses the framing into the same
  shape GMCP produces and dispatches through the *same* Lua surface
  (an `"msdp"` hook mirroring `"gmcp"`, or normalized into one
  `"oob"` event - decide when implementing). Servers that offer both
  GMCP and MSDP get GMCP; scripts shouldn't care which transport fed
  them.
- **MSSP:** server status (name, players, uptime) - read-only
  crawler protocol. Trivial once MSDP framing exists (same
  `VAR`/`VAL` grammar); surfaces as a `"mssp"` notification hook and
  feeds nice `/world` metadata.

---

## Phase 5: Presentation Protocols

Lower priority: fewer servers require them, and each has real UI cost.

### MXP (option 91)

Inline markup: clickable links, colors, entity expansion. Staged
deliberately:

1. **Parse-and-strip** - negotiate MXP, strip tags so MXP-heavy MUDs
   render clean text. Correctness win, zero UI work.
2. **Links + color tags** - `<a>`/`<send>` become clickable spans (the
   TUI needs a span/metadata model on `text.Line` for this; that Line
   change is the real cost, and also what OSC 8 hyperlinks would use).
3. **Full entity/element support** - only by demand; most modern MUDs
   that want rich data use GMCP instead.

### MSP (option 90)

Sound triggers (`!!SOUND(...)`). Go recognizes and strips the
directives, emits a `"sound"` hook event; whether/how to play is Lua
policy shelling out to a player. Cheap once the pattern exists; ship
with a disabled-by-default example script.

---

## Client Extensions (not protocols)

Features major clients ship that fit rune's Lua-space model. Each of
these should be possible *as a user script first*; core adoption only
when a primitive is genuinely missing.

| Extension | What's missing today | Home |
|---|---|---|
| **Auto-login** | Nothing - worlds already store an opts table verbatim; define `on_connect` (and decide the credentials story: plaintext in `store.json` vs. a keyring hook) | Lua core (`58_worlds.lua`) |
| **Speedwalking** | Nothing - `#N` repeats exist; `.nnesw`-style parsing is a small input hook | Example script |
| **Keepalive / anti-idle** | Nothing - `rune.timer.every` + configurable command | Example script |
| **Mapper** | GMCP Room.Info (Phase 3) + a pane; graph + drawing is pure Lua | Lua package, post-GMCP |
| **Scrollback search** | A UI primitive (`rune._ui.search` / viewport find) - binds and UX in Lua | Go primitive + Lua |
| **TTS / accessibility** (blightmud's standout) | A `rune._tts.speak` primitive wrapping a platform TTS engine; which lines to speak is a Lua hook policy | Go primitive + Lua |
| **Script package layout** | Convention only: `~/.config/rune/scripts/<pkg>/init.lua` + `require`; a `/pkg` installer is far-future | Docs first |
| **Plugin distribution** | Not planned until the script ecosystem exists | - |

## Explicit Non-Goals

- **Multi-session** - contradicts the single-Session actor model; run
  two runes.
- **Pueblo, ZMP, ATCP1, Aardwolf 102** - superseded or single-server
  protocols; only by concrete demand (102 is a thin GMCP-like frame if
  Aardwolf players ask).
- **Inter-client chat** (TinTin++ `#chat`, MudMaster chat) - niche,
  large surface, better served by actual chat software.

## Sequencing Summary

```
1. TTYPE/MTTS → NAWS → CHARSET → MNES     ✅ shipped (identity: unblocks server-side features)
2. MCCP2                                   ✅ shipped (MCCP3 remains future)
3. GMCP + Lua rune.gmcp surface            ✅ shipped (reuses store's JSON bridge)
4. MSDP → MSSP                             (same Lua surface, alternate transport)
5. MXP strip → links; MSP                  (presentation; gated on text.Line span model)
   Extensions ride alongside: auto-login and TTS have no protocol dependency;
   the mapper is unblocked now that GMCP has shipped.
```

Each phase ends the same way: option advertised only when implemented,
loopback + parser fixtures in tests, `docs/lua_doc.md` updated for any
new Lua surface, and a real-server smoke test noted in the PR.
