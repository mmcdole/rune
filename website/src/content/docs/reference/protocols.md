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
| SGA / EOR | 3 / 25 | Prompt detection modes |
| TTYPE / MTTS | 24 | Reports `RUNE`, terminal type, and an honest MTTS bitvector (ANSI, VT100, UTF-8, 256 colors, truecolor, MNES; bit 2048 on TLS connections) |
| NAWS | 31 | Window size, re-sent on every resize |
| NEW-ENVIRON / MNES | 39 | `CLIENT_NAME`, `CLIENT_VERSION`, `CHARSET`, `MTTS`, `TERMINAL_TYPE` |
| CHARSET | 42 | Accepts UTF-8, rejects everything else |
| MCCP2 | 86 | zlib decompression; a clean stream end resumes plain telnet |
| GMCP | 201 | Framing and JSON in Go; handlers and `Core.Supports` policy in Lua ([rune.gmcp](/rune/scripting/gmcp/)) |

**TLS** is address-level rather than a telnet option: `tls://host:port`
verifies certificates, and `tls+insecure://host:port` accepts self-signed
ones (common on MUDs). Plain telnet is the default; `telnet://` is
accepted explicitly.

Refused (not implemented): MCCP3, MSSP, ZMP, MXP, MSP, LINEMODE. The
[roadmap](https://github.com/mmcdole/rune/blob/main/docs/future.md) covers
what's next: MSDP/MSSP, then MXP in stages.

**Related:** [GMCP](/rune/scripting/gmcp/)
