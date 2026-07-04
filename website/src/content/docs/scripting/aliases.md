---
title: Aliases
description: Expand what you type before it reaches the server, with exact words or regex patterns and string or function actions.
---

An alias rewrites what you type before it goes to the server. Two decisions
define one: how it matches (an exact command word, or a regex over the
whole line) and what it does (a string to send, or a Lua function to run).

The simplest form is a word that expands, with anything you typed after it
carried along:

```lua
rune.alias.exact("gc", "get all from corpse")
```

Type `gc` and the server receives `get all from corpse`. Type `gc bag` and
it receives `get all from corpse bag`. Arguments are appended.

When the alias needs logic, such as a target variable or a condition, the
action is a function instead:

```lua
rune.alias.exact("heal", function(args)
    rune.send("cast 'heal' " .. (args ~= "" and args or "self"))
end)
```

Both forms register the same way and are managed the same way.

## Creating

```lua
rune.alias.exact(word, action, opts?)     -- matches the first word you type
rune.alias.regex(pattern, action, opts?)  -- Go regexp against the whole line
```

Exact aliases are an O(1) lookup on the command word; use them for most
cases. Regex aliases see the entire input line and can capture pieces of
it. They run in `priority` order before exact aliases are tried.

Regex patterns are validated at registration: a bad pattern raises
immediately instead of failing silently at match time.

## Actions

**A string** is a plain expansion.

- For `exact` aliases, whatever you typed after the word is appended:
  `rune.alias.exact("k", "kill")` turns `k rat` into `kill rat`.
- For `regex` aliases, `%1`, `%2`, and so on are substituted from the
  pattern's captures. This is how you reorder or reuse arguments:

```lua
rune.alias.regex("^gr (.+)$", "get %1;wear %1")
-- "gr helmet" -> get helmet;wear helmet
```

**A function** runs instead of sending anything. Send what you want with
`rune.send`. Return a string to feed it back through expansion (so aliases
can build on other aliases), or return nothing to consume the input
entirely.

The function's first argument depends on the match type:

```lua
-- exact: (args, ctx). args is the text after the command word.
rune.alias.exact("heal", function(args, ctx)
    if args == "" then
        rune.echo(rune.style.yellow("heal who?"))
    else
        rune.send("cast 'heal' " .. args)
    end
end)

-- regex: (matches, ctx). matches is the array of captures.
rune.alias.regex("^kk (\\w+)$", function(matches, ctx)
    rune.send("kill " .. matches[1])
end)
```

`ctx` carries `line` (the full input), plus `name`, `group`, and `type`.

Start with a string. Switch to a function when you need state or a
condition.

## Options

Aliases take the [common options](/scripting/model/#options): `name`,
`group`, `priority` (order among regex aliases), and `once`. No
alias-specific extras.

## Examples

Chaining and repeats compose with aliases, since expansion runs on the
result:

```lua
-- "farm" runs six kill/loot rounds, via #N repeat syntax
rune.alias.exact("farm", "#6 {kill rat;loot}")
```

Captures beyond the first:

```lua
-- "g 20 bob" -> "give 20 gold bob"
rune.alias.regex("^g (\\d+) (\\w+)$", "give %1 gold %2")
```

State shared between two aliases:

```lua
local last_target
rune.alias.regex("^kk (\\w+)$", function(matches)
    last_target = matches[1]
    rune.send("kill " .. last_target)
end)
rune.alias.exact("again", function()
    if last_target then rune.send("kill " .. last_target) end
end)
```

Grouped, so a pack can be switched off mid-fight:

```lua
rune.alias.exact("n", "sneak north", { group = "sneaky" })
rune.alias.exact("s", "sneak south", { group = "sneaky" })
-- /group sneaky off  -> n and s pass through unchanged
```

## Managing

Every constructor returns a handle:

```lua
local h = rune.alias.exact("k", "kill", { name = "quick-kill" })
h:disable()  h:enable()  h:remove()
```

By name: `rune.alias.disable/enable/remove(name)` — the full management
suite is in the [API reference](/reference/api/#managing). In the client,
`/aliases` shows every alias with its state, group, and the `file:line`
that registered it.

## Gotchas

- `%N` capture substitution only works in regex aliases. An exact alias's
  string action gets the arguments appended at the end; to place them in
  the middle, use `regex` or a function.
- Patterns are Go regexp (RE2), not Lua patterns: `\\d` and `\\w` work,
  backreferences do not. Test a pattern with
  `/lua rune.echo(tostring(rune.regex.match("^k (.+)$", "k rat")[1]))`.
- An alias that errors three times in a row is
  [quarantined](/scripting/model/#quarantine).
- Alias expansion recurses (an alias can produce another alias's input)
  with a depth limit of 100 to catch loops.

**Related:** [rune.alias reference](/reference/api/alias/),
[Triggers](/scripting/triggers/),
[Groups](/scripting/groups/)
