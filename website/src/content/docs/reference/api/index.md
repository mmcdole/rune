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
| `rune.bind` | [rune.bind](/reference/api/bind/) | Key binding registration and management |
| `rune.command` | [rune.command](/reference/api/command/) | Custom `/commands` |
| `rune.group` | [rune.group](/reference/api/group/) | Batch enable/disable across registries |
| `rune.gmcp` | [rune.gmcp](/reference/api/gmcp/) | GMCP handlers, sending, subscriptions |
| `rune.http` | [rune.http](/reference/api/http/) | Async HTTP requests with callbacks |
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

For JSON request bodies, responses, and session state, see
[`rune.json`](/reference/api/json/).

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
| `h:action()` | The registered action: the function, or the string for string actions |

Methods are chainable. See
[The Scripting Model](/scripting/model/#handles) for usage.

`h:action()` returns the callback as registered, so calling it runs
neither the enabled and group checks nor the failure quarantine. It is
for capturing an existing action and wrapping it, not for dispatch.

## Names

A registration's name addresses it in
[`get`, `enable`, `disable` and `remove`](#managing), replacement registrations,
and listing commands. Registrations with a natural key are addressed by
that key as well:

| Creation function | Addressed by | Because |
|---|---|---|
| `rune.trigger.*` | the `name` you give it | several triggers can match one line |
| `rune.alias.regex` | the `name` you give it | several can match one line |
| `rune.timer.*` | the `name` you give it | nothing about a timer is unique |
| `rune.hooks.on` | the `name` you give it | several handlers per event |
| `rune.gmcp.on` | the `name` you give it | several handlers per package |
| `rune.bind` | the key, `"ctrl+g"`, and any `name` you give it | one bind per key |
| `rune.alias.exact` | the phrase, `"chat off"`, and any `name` you give it | one expansion per typed phrase |
| `rune.ui.bar` | the bar name, `"status"` | one renderer per named bar |
| `rune.command.add` | the command, `"greet"` | one handler per `/command` |

Within a registry, each address identifies one registration. Registering on
a taken key or name replaces that registration and releases its other
address. A name that spells another registration's key, or a key that
spells another's name, is an error and nothing changes.

```lua
rune.trigger.contains("food", "eat bread", { name = "feeder" })
rune.bind("ctrl+g", toggle_map, { name = "map" })

rune.trigger.disable("feeder")
rune.binds.disable("map")     -- the same bind as rune.binds.disable("ctrl+g")
rune.bars.get("status")       -- the core's own bar, registered without options
```

A bind or exact alias registered without a name reports its key from
`h:name()` and in listings. Bars and commands take no `name`; one is
ignored with a notice.

## Options

Common `opts` fields accepted by every creation function:

| Option | Type | Default | Applies to |
|---|---|---|---|
| `group` | string | none | All: membership for batch operations |
| `priority` | number | 50 | Regex aliases, triggers, hooks. Lower runs first |
| `once` | bool | false | Aliases, triggers. Remove after first match |

`name` names the registration, except for bars and commands; see
[Names](#names). Page-specific extras (e.g. trigger `gag`/`raw`) are
listed on each page.

## Managing

Every registry namespace exposes the same functions, each taking the
item's name:

| Function | Effect |
|---|---|
| `.get(name)` | The item's handle, or nil |
| `.enable(name)` / `.disable(name)` | Enable or disable an item |
| `.remove(name)` | Unregister an item |
| `.list()` | All items with name, enabled state, group, and source `file:line` |
| `.count()` | Number of registered items |
| `.clear()` | Remove everything in the registry |
| `.remove_group(group)` | Remove all items in a group |

The bar registry also exposes `.toggle(name)`, returning whether that bar
exists. See [rune.ui](/reference/api/ui/#managing-bars).

The matching slash commands — `/triggers`, `/aliases`, `/timers`,
`/hooks`, `/binds`, `/bars` — print the same listings.

## Quarantine

A callback that errors 3 times consecutively is disabled individually,
with a notice. Re-enable it (`.enable(name)`), re-register it, or
`/reload` to reset the count; one successful run also clears it. See
[The Scripting Model](/scripting/model/#quarantine).

**Related:** [The Scripting Model](/scripting/model/) ·
[Slash Commands](/reference/slash-commands/) ·
[Protocols](/reference/protocols/)
