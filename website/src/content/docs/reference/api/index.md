---
title: Lua API Overview
description: Every rune.* namespace, and the contracts they all share.
---

The complete public scripting API, one page per namespace. For a
task-oriented introduction, start with
[Scripting Basics](/getting-started/scripting-basics/) and
[The Scripting Model](/scripting/model/).

| Namespace | Page | What it does |
|---|---|---|
| `rune.send`, `rune.connect`, … | [Core](/reference/api/core/) | Sending, connecting, loading scripts, quitting |
| `rune.state`, `rune.line` | [State & Lines](/reference/api/state-lines/) | Read-only client state; the line object contract |
| `rune.style` | [rune.style](/reference/api/style/) | ANSI color and attribute helpers |
| `rune.regex` | [rune.regex](/reference/api/regex/) | Go-regexp matching, validation, compilation |
| `rune.trigger` | [rune.trigger](/reference/api/trigger/) | React to server output |
| `rune.alias` | [rune.alias](/reference/api/alias/) | Expand and transform your input |
| `rune.timer` | [rune.timer](/reference/api/timer/) | One-shot and repeating timers |
| `rune.hooks` | [rune.hooks](/reference/api/hooks/) | Event handlers, plus the full event catalog |
| `rune.bind` | [rune.bind](/reference/api/bind/) | Key bindings, plus the default keymap |
| `rune.command` | [rune.command](/reference/api/command/) | Custom `/commands` |
| `rune.group` | [rune.group](/reference/api/group/) | Batch enable/disable across registries |
| `rune.gmcp` | [rune.gmcp](/reference/api/gmcp/) | GMCP handlers, sending, subscriptions |
| `rune.input`, `rune.history` | [rune.input](/reference/api/input/) | The input line and command history |
| `rune.session`, `rune.store`, `rune.world` | [Storage](/reference/api/storage/) | Session and durable storage; world bookmarks |
| `rune.log` | [rune.log](/reference/api/log/) | Session logging |
| `rune.ui` | [rune.ui](/reference/api/ui/) | Layout, bars, bar management |
| `rune.ui.picker` | [rune.ui.picker](/reference/api/picker/) | Fuzzy-filter selection panels |
| `rune.pane` | [rune.pane](/reference/api/pane/) | Scrollable text panes |

Also in Reference: the built-in
[slash commands](/reference/slash-commands/) and the
[protocols](/reference/protocols/) rune negotiates on the wire.

The contracts below apply across the API; individual pages link here
rather than restating them.

## Handles

Every creation function (`rune.trigger.*`, `rune.alias.*`,
`rune.timer.*`, `rune.hooks.on`, `rune.bind`, `rune.ui.bar`,
`rune.gmcp.on`, `rune.command.add`) returns a handle:

| Method | Effect |
|---|---|
| `h:enable()` / `h:disable()` | Toggle without unregistering |
| `h:remove()` | Unregister (timers also accept `h:cancel()`) |
| `h:name()` | The item's name, or nil |
| `h:group()` | The item's group, or nil |

Methods are chainable. See
[The Scripting Model](/scripting/model/#handles) for usage.

## Options

Common `opts` fields accepted by every creation function:

| Option | Type | Default | Applies to |
|---|---|---|---|
| `name` | string | none | All — unique ID; same name replaces (upsert) |
| `group` | string | none | All — membership for batch operations |
| `priority` | number | 50 | Aliases, triggers, hooks — lower runs first |
| `once` | bool | false | Aliases, triggers — remove after first match |

Page-specific extras (e.g. trigger `gag`/`raw`) are listed on each page.

## Managing

Every registry namespace exposes the same management suite, addressed
by item name:

| Function | Effect |
|---|---|
| `.enable(name)` / `.disable(name)` | Toggle an item |
| `.remove(name)` | Unregister an item |
| `.list()` | All items with name, enabled state, group, and source `file:line` |
| `.count()` | Number of registered items |
| `.clear()` | Remove everything in the registry |
| `.remove_group(group)` | Remove all items in a group |

The matching slash commands — `/triggers`, `/aliases`, `/timers`,
`/hooks`, `/binds`, `/bars` — print the same listings.

## Quarantine

A callback that errors 3 times consecutively is disabled individually,
with a notice. Re-enable it (`.enable(name)`), re-register it, or
`/reload` to reset the count; one successful run also clears it. Full
story: [The Scripting Model](/scripting/model/#quarantine).

**Related:** [The Scripting Model](/scripting/model/) ·
[Slash Commands](/reference/slash-commands/) ·
[Protocols](/reference/protocols/)
