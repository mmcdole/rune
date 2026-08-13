---
title: Protocols
description: What rune negotiates on the wire, and the policy behind it.
---

Rune advertises only what it implements. Agreeing to an option and then
ignoring its subnegotiations breaks real servers, so the compatibility
table and the feature list are the same list.

| Protocol | Option | What rune does |
|---|---|---|
| ECHO | 1 | Server-controlled local echo (passwords hidden) |
| SGA / EOR | 3 / 25 | Telnet character and record framing; GA/EOR confirms a non-empty current prompt |
| TTYPE / MTTS | 24 | Reports `RUNE`, terminal type, and an honest MTTS bitvector (ANSI, VT100, UTF-8, 256 colors, truecolor, MNES; bit 2048 on TLS connections) |
| NAWS | 31 | Window size, re-sent on every resize |
| NEW-ENVIRON / MNES | 39 | `CLIENT_NAME`, `CLIENT_VERSION`, `CHARSET`, `MTTS`, `TERMINAL_TYPE` |
| CHARSET | 42 | Accepts UTF-8, rejects everything else |
| MCCP2 | 86 | zlib decompression; a clean stream end resumes plain telnet |
| GMCP | 201 | Framing and JSON in Go; handlers and `Core.Supports` policy in Lua ([rune.gmcp](/scripting/gmcp/)) |

**TLS** is address-level rather than a telnet option: `tls://host:port`
verifies certificates, and `tls+insecure://host:port` accepts self-signed
ones (common on MUDs). Plain telnet is the default; `telnet://` is
accepted explicitly.

Refused (not implemented): MCCP3, MSSP, ZMP, MXP, MSP, LINEMODE.

## Ordering and prompt boundaries

Rune does not guess prompts from text patterns or timers. Server text ending in
a newline or bare CR is ordinary output. Unterminated text is exposed through
the `prompt` hook with `confirmed = false`; a following Telnet GA or EOR
consumes it through the same hook with `confirmed = true`. When unterminated
data and its GA/EOR arrive in one parser batch, Rune emits only the confirmed
observation. When they arrive in separate batches, scripts may first observe
the growing unterminated text and later its confirmation. CR terminates a line
immediately, including at the end of an event or batch; an optional following
LF is swallowed even when it arrives in a later event or batch.

The transport consumes MCCP activation events itself, then sends any remaining
Session-facing events from one parser result as one owned,
connection-tagged batch. An MCCP-only result sends no batch. The transport owns
TCP/TLS, parser framing, MCCP decompression, and the single socket writer.
Session owns the current server line and one connection-specific Telnet
protocol reducer. It processes a batch synchronously, including Lua callbacks
and the frames they send, so TCP chunking cannot reorder negotiation effects.

GMCP follows that same wire order. A server's `WILL GMCP` queues Rune's `DO`,
enables GMCP, and runs the `gmcp_enabled` policy that sends `Core.Hello` and
the configured support set when it is non-empty. Later payloads in the batch
are then delivered to handlers. A subsequent `WONT GMCP` queues `DONT` and
disables GMCP; Rune does not discard an earlier payload merely because the
batch ends disabled. Outbound game lines and protocol frames are checked and
queued against the connection that produced the callback, preventing a
reconnect from redirecting an old write to a new server.

**Related:** [GMCP](/scripting/gmcp/)
