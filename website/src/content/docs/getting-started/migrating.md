---
title: Migrating from Other Clients
description: A translation map from TinTin++ and Mudlet syntax to rune's Lua API.
---

Rune has no in-client command language: scripting is Lua. Aliases, triggers,
and timers live in `~/.config/rune/init.lua` (auto-loaded at startup), and the
edit loop is: change the file, `/reload`, keep playing. `/lua <code>` runs
one-liners without touching a file.

A trigger you'd write like this in TinTin++:

```txt
#action {%1 attacks you} {flee}
```

looks like this in `init.lua`:

```lua
rune.trigger.regex("(\\w+) attacks you", "flee")
```

## From TinTin++

| TinTin++ | Rune equivalent |
|---|---|
| `#alias {k} {kill}` | `rune.alias.exact("k", "kill")`; typed args are appended, so `k rat` sends `kill rat` |
| `#alias {gr %1} {get %1;wear %1}` | `rune.alias.regex("^gr (.+)$", "get %1;wear %1")` |
| `#action {%1 attacks you} {flee}` | `rune.trigger.regex("(\\w+) attacks you", "flee")` |
| `#gag {spam line}` | `rune.trigger.contains("spam line", nil, { gag = true })` |
| `#highlight {gold} {yellow}` | `rune.trigger.contains("gold", function(m, ctx) return rune.style.yellow(ctx.line:clean()) end)` |
| `#substitute` | a trigger action returning the rewritten string |
| `#tick` / `#ticker` | `rune.timer.every(60, "save", { name = "tick" })` |
| `#delay {5} {stand}` | `rune.timer.after(5, "stand")` |
| `#var` | plain Lua variables (`rune.session`/`rune.store` for state that must survive `/reload`/restarts) |
| `#if`, `#math`, `#loop` | Lua (`if`, arithmetic, `for`) |
| `#showme` | `rune.echo(text)` |
| `#split` | `rune.ui.layout{...}` + bars |
| `#session x host port` | `/connect host port`, `/world add` for bookmarks |
| `#read file.tin` | `/load file.lua` |
| `#write` | your scripts are already files |
| `#3 north` | works as-is: `#3 north`, `#3 {kill rat;loot}` |
| `;` separator | works as-is: `kill rat;loot` |

## From Mudlet

The scripting language is the same; the API names differ:

| Mudlet | Rune equivalent |
|---|---|
| `send("kill rat")` | `rune.send("kill rat")` |
| `echo("text")` / `cecho(...)` | `rune.echo(text)`, colored via `rune.style.*` |
| `matches[2]` (first capture) | `matches[1]`; there is no whole-match slot |
| `tempTimer(5, code)` | `rune.timer.after(5, action)` |
| `tempRegexTrigger(pattern, code)` | `rune.trigger.regex(pattern, action)` |
| script editor window | your `$EDITOR`; `Ctrl+E` edits the input line in it too |

## From MUSHclient

The scripting language is the same; the API names differ. If you're using
MUSHclient's XML syntax for creating triggers, aliases, and timers, you'll
need to covert those to Lua first.

| MUSHclient                        | Rune equivalent                       |
|-----------------------------------|---------------------------------------|
| `Send("kill rat")`                | `rune.send_raw("kill rat")`           |
| `Execute("kill rat")`             | `rune.send("kill rat")`               |
| `Note("text")`                    | `rune.echo(text)`                     |
| `wildcards[1]`                    | `matches[1]`                          |
| `DoAfter(5, "drink")`             | `rune.timer.after(5, "drink")`        |
| `tempRegexTrigger(pattern, code)` | `rune.trigger.regex(pattern, action)` |
| `EnableTrigger("tname",false)`    | `rune.trigger.disable("tname")`       |
| `SetVariable("varname","value")`  | `rune.store.set("varname","value")`   |

Some functions such as ColourNote have no direct equivalent, but a simple
wrapper function could replace it with Lua. (using `rune.style.*`)

## Capture references

Regex aliases and triggers substitute `%1`, `%2`, and so on from captures
in string actions, so most one-liners port verbatim. Function actions
receive `(matches, ctx)` when you need logic:

```lua
rune.trigger.regex("^(\\w+) tells you '(.+)'$", function(matches)
    rune.echo(rune.style.cyan("TELL from " .. matches[1]))
end)
```

## What carries over unchanged

`;` chaining, `#N` repeats, `/commands`, up-arrow history,
Ctrl+R history search, and tab completion all behave the way
terminal-client muscle memory expects.

## Where things live

Everything lives under `~/.config/rune/` — the full path table is in
[Installation](/getting-started/installation/#where-things-live). Your
scripts go in `init.lua`; bookmarks and durable state in `store.json`.

Full signatures for every `rune.*` namespace:
[API reference](/reference/api/).

## Next

[Triggers](/scripting/triggers/) is where most ports start — match
modes, captures, gagging, and rewriting in rune's terms.
