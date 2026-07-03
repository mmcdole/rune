# Coming from TinTin++ or Mudlet

Rune takes a different approach from tintin: there is no in-client
command language. **Lua is the command language.** Aliases, triggers,
and timers are defined in `~/.config/rune/init.lua` (auto-loaded at
startup), and the edit loop is: change the file, `/reload`, keep
playing. `/lua <code>` runs one-liners without touching a file.

If you're used to typing `#alias {k} {kill %1}` mid-game, the trade is
this: you lose one-command registration, and you gain a real language
(functions, state, modules) with the same line count for simple cases
and much less pain for complex ones.

## Command map

| You know | Rune equivalent |
|---|---|
| `#alias {k} {kill %1}` | `rune.alias.exact("k", "kill %1")` |
| `#alias {^gr (.+)$} {get %1;wear %1}` | `rune.alias.regex("^gr (.+)$", "get %1;wear %1")` |
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
| tintin: `#3 north` | works as-is: `#3 north`, `#3 {kill rat;loot}` |
| tintin: `;` separator | works as-is: `kill rat;loot` |
| Mudlet: script editor | your `$EDITOR`; `Ctrl+E` edits the input line in it too |

## Capture references

Regex aliases and triggers substitute `%1`-`%9` from captures, so most
tintin one-liners port verbatim. Function actions receive
`(matches, ctx)` when you need logic:

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

| | |
|---|---|
| Config / scripts | `~/.config/rune/init.lua` (+ anything you `require`) |
| Bookmarks & durable state | `~/.config/rune/store.json` (managed by `/world` and `rune.store`) |
| Logs | `~/.config/rune/logs/` (via `/log`) |

Full API: [lua_doc.md](lua_doc.md).
