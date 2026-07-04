---
title: rune.alias
description: Full signatures for expanding and transforming your input — exact word and regex matching.
---

Aliases match your input and transform or expand it before it reaches
the server. For a task-oriented introduction, see
[Aliases](/scripting/aliases/).

## Quick reference

```lua
rune.alias.exact(command, action, opts?)  -- first word matches literally
rune.alias.regex(pattern, action, opts?)  -- Go regexp on the full input line
```

Both constructors return a [handle](/reference/api/#handles) and accept
the [common options](/reference/api/#options).

## Matching

Regex aliases are checked first, in `priority` order; if none match,
the first word of the input is looked up among exact aliases. Only one
alias fires per command. A string result — whether from a string action
or returned by a function — is fed back through
[`rune.send`](/reference/api/core/), so aliases can expand to other
aliases; a depth limit catches loops.

### rune.alias.exact

```lua
rune.alias.exact(command, action, opts?) -> handle
```

- `command` (string) — matched literally against the first word of the
  input. Registering the same word again replaces the previous exact
  alias.
- `action` (string | function) — an expansion string (trailing
  arguments are appended: with `rune.alias.exact("g", "get")`, typing
  `g sword` sends `get sword`), or `function(args, ctx)` where `args`
  is everything after the command word.
- `opts` (table, optional) — [common options](/reference/api/#options).

```lua
rune.alias.exact("heal", function(args, ctx)
    rune.send("cast heal " .. (args ~= "" and args or "self"))
end)
```

### rune.alias.regex

```lua
rune.alias.regex(pattern, action, opts?) -> handle
```

- `pattern` (string) — Go regexp ([RE2](/reference/api/regex/), not Lua
  patterns), matched against the full input line. Validated at
  registration; a bad pattern raises immediately.
- `action` (string | function) — a command string (`%1`…`%n`
  substituted from captures), or `function(matches, ctx)` where
  `matches` is the capture array.
- `opts` (table, optional) — [common options](/reference/api/#options).

```lua
-- "give 5 coins to bob" → sends "give coins bob" 5 times
rune.alias.regex("^give\\s+(\\d+)\\s+(\\w+)\\s+to\\s+(\\w+)", function(m)
    for i = 1, tonumber(m[1]) do
        rune.send("give " .. m[2] .. " " .. m[3])
    end
end)
```

## Actions and return values

A string action is the replacement command. A function action receives
`(args, ctx)` for exact aliases or `(matches, ctx)` for regex aliases —
see [the context object](/scripting/model/#the-context-object) — and
its return value controls what happens next: return a string to have
it processed and sent in place of the input, or return nothing to
consume the input entirely (the function already did the work).

## Managing

Standard registry management applies:
`rune.alias.enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)` — see
[Registries](/reference/api/#managing). `/aliases` lists everything.

**Related:** [Aliases guide](/scripting/aliases/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.regex](/reference/api/regex/) · [Core](/reference/api/core/)
