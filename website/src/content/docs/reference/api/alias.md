---
title: rune.alias
description: Full signatures for expanding and transforming your input — literal phrase and regex matching.
---

Aliases match your input and transform or expand it before it reaches
the server. For a task-oriented introduction, see
[Aliases](/scripting/aliases/).

## Quick reference

```lua
rune.alias.exact(phrase, action, opts?)   -- leading command phrase matches literally
rune.alias.regex(pattern, action, opts?)  -- Go regexp on the full input line
rune.alias.get(name)                      -- the alias's handle, or nil
```

Both constructors return a [handle](/reference/api/#handles) and accept
the [common options](/reference/api/#options). They differ in how they
are [named](/reference/api/#names): an exact alias is named for its
normalized phrase, so `rune.alias.disable("chat off")` addresses it and a
`name` in `opts` is ignored with a notice. A regex alias is a matcher
rather than a phrase, and several can match one line, so it takes `name`
as usual.

## Matching

Regex aliases are checked first, in `priority` order. If none match, an
exact alias can match one or more complete words at the beginning of what
you type. When more than one matches, the longest active phrase wins, so
`"chat off"` takes precedence over `"chat"`. Only one alias fires per
command. A string result — whether from a string action or returned by a
function — is fed back through
[`rune.send`](/reference/api/core/), so aliases can expand to other
aliases; a depth limit catches loops.

### rune.alias.exact

```lua
rune.alias.exact(phrase, action, opts?) -> handle
```

- `phrase` (string) — one or more words matched literally at the start of
  the input. Whitespace separates words and is normalized, so `chat off`
  also matches `chat   off`. Registering the same normalized phrase again
  replaces the previous exact alias.
- `action` (string | function) — an expansion string (trailing
  arguments are appended: with `rune.alias.exact("g", "get")`, typing
  `g sword` sends `get sword`), or `function(args, ctx)` where `args`
  is everything after the matched phrase.
- `opts` (table, optional) — [common options](/reference/api/#options).

```lua
rune.alias.exact("chat off", function(args, ctx)
    rune.echo("chatlog: off" .. (args ~= "" and " (" .. args .. ")" or ""))
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

## Default aliases

The core registers one alias, named `repeat-last`: shell-style history
expansion, where `!` or `!!` resends the last command and `!prefix`
resends the newest command starting with `prefix`. History keeps the
expanded command, not the bang line. Remove it with
`rune.alias.remove("repeat-last")` if your game uses `!` as a real
command; verbatim submissions always bypass aliases.

## Managing

Standard registry management applies:
`rune.alias.get/enable/disable/remove(name)`, `.list()`, `.count()`,
`.clear()`, `.remove_group(group)`. See
[Registries](/reference/api/#managing). `/aliases` lists everything.

**Related:** [Aliases guide](/scripting/aliases/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.regex](/reference/api/regex/) · [Core](/reference/api/core/)
