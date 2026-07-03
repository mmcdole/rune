---
title: Slash Commands
description: Every built-in /command.
---

`/help` shows this list in the client, including any commands your scripts
add. `/` on an empty line opens the fuzzy picker over the same registry.

## Connection

| Command | Description |
|---|---|
| `/connect` | No args: picker over saved worlds |
| `/connect <world>` | Connect to a bookmark by name |
| `/connect <host> <port> [tls\|tls+insecure]` | Connect directly |
| `/connect <host:port>` | Address form; schemes `tls://`, `tls+insecure://` |
| `/disconnect` | Close the connection |
| `/reconnect` | Redial the last server (stored durably; survives `/reload` and restarts) |
| `/world add <name> <host> <port> [tls\|tls+insecure]` | Save a bookmark (also accepts a `host:port` address) |
| `/world remove <name>` | Delete a bookmark |
| `/world` / `/world list` / `/worlds` | List bookmarks |

## Scripts

| Command | Description |
|---|---|
| `/load <path>` | Load a Lua file |
| `/reload` | Rebuild the Lua VM and reload everything |
| `/lua <code>` | Run Lua inline; a non-`nil` result is printed |
| `/test <line>` | Simulate a server line through your triggers |

## Introspection

| Command | Description |
|---|---|
| `/aliases` `/triggers` `/timers` `/hooks` `/binds` `/bars` | List registrations with state, group, and source `file:line` |
| `/groups` | List groups and their state |
| `/group <name> on\|off` | Toggle a group |
| `/gmcp` | GMCP negotiation state, subscriptions, handlers |
| `/gmcp send <package> [json]` | Send a raw GMCP message |
| `/help` | List all commands, including script-added ones |

## Session

| Command | Description |
|---|---|
| `/log start [file]` / `/log stop` / `/log status` | Session logging; bare `/log` shows status |
| `/raw <text>` | Send without alias expansion |
| `/echo <text>` | Print locally, never sent |
| `/version` | Client version |
| `/quit` | Exit |

**Related:** [Slash Commands](/rune/scripting/commands/) for adding your own

