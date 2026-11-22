---
title: Protocols
description: The telnet and terminal protocols rune supports.
---

## Telnet

| Protocol | Option | What rune does |
|---|---|---|
| ECHO | 1 | Server-controlled local echo, so passwords stay hidden |
| SGA / EOR | 3 / 25 | GA and EOR mark the end of a prompt; see [prompt semantics](/scripting/hooks/#prompt-semantics) |
| TTYPE / MTTS | 24 | Reports `RUNE`, the terminal type, and MTTS flags: ANSI, VT100, UTF-8, 256 colors, truecolor, MNES, and TLS on TLS connections |
| NAWS | 31 | Sends the window size, and sends it again on every resize |
| NEW-ENVIRON / MNES | 39 | `CLIENT_NAME`, `CLIENT_VERSION`, `CHARSET`, `MTTS`, `TERMINAL_TYPE` |
| CHARSET | 42 | UTF-8 only |
| MCCP2 | 86 | Compression |
| GMCP | 201 | Handled from Lua; see [GMCP](/scripting/gmcp/) |

Not supported: MXP, MSP, MSDP, MSSP, MCCP3, ZMP, LINEMODE. Rune declines
these when a server offers them.

### TLS

Connect with `tls://host:port` for a verified TLS connection, or
`tls+insecure://host:port` to accept a self-signed certificate, which is
common on MUDs. Plain telnet is the default; `telnet://` also works.

## Terminal

| Protocol | How rune uses it |
|---|---|
| ANSI SGR color | 16, 256, and truecolor |
| Kitty keyboard protocol | Unambiguous key reporting; see [key bindings](/scripting/keybindings/) |
| xterm modifyOtherKeys | Fallback for terminals without the Kitty protocol |
| DEC application keypad (DECKPAM) | Numpad keys, with the `numpad` setting; see [numpad keys](/scripting/keybindings/#numpad-keys) |
| xterm mouse (SGR) | Wheel scrolling, with the `mouse` setting |
| Bracketed paste | Multi-line paste |
| OSC 52 | Clipboard; see [`rune.clipboard`](/reference/api/clipboard/) |
| Synchronized output (mode 2026) | Flicker-free redraws, when the terminal supports it |
| Unicode core (mode 2027) | Grapheme-cluster widths for emoji and CJK text |

Not supported: Kitty graphics, Sixel, iTerm2 inline images, OSC 8 hyperlinks.
