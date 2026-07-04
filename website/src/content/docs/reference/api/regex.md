---
title: rune.regex
description: Go-regexp matching, validation, and compilation — plus the pattern syntax rune uses everywhere.
---

The regexp engine behind `trigger.regex` and `alias.regex`, exposed
directly for your own matching. For where regexes fit in practice, see
[Triggers](/scripting/triggers/).

## Quick reference

```lua
rune.regex.match(pattern, text)  -- captures array, or nil (cached)
rune.regex.validate(pattern)     -- true, or nil + error message
rune.regex.compile(pattern)      -- compiled object, or nil + error
```

## Pattern syntax

Rune uses Go's regexp engine ([RE2](https://github.com/google/re2/wiki/Syntax)),
**not** Lua patterns:

- `\\w`, `\\s`, `\\d` work; Lua's `%w`, `%s` don't.
- Captures use parentheses: `(\\w+)`. Non-capturing: `(?:\\w+)`.
- No backreferences or lookaround (RE2 guarantees linear-time matching
  instead).
- Escape backslashes in Lua strings: write `"\\d+"` to match digits.

`rune.trigger.regex` and `rune.alias.regex` validate their patterns
eagerly at registration and raise on a bad one, so typos fail loudly
instead of silently never matching.

### rune.regex.match

```lua
rune.regex.match(pattern, text) -> captures | nil
```

- `pattern` (string) — Go regexp.
- `text` (string) — text to match against.

Returns an array of the **captured groups only** — the full match is
not included — or `nil` if the pattern doesn't match. A pattern with
no capture groups returns an empty table on a match:

```lua
local caps = rune.regex.match("^(\\w+)\\s+(\\d+)", "foo 42")
-- caps = {"foo", "42"}
```

Compiled patterns are cached, so calling `match` with the same pattern
on every line is cheap. The cache is bounded — at 512 entries it resets
and patterns recompile on next use — so a stable set of patterns never
hits the cap. An invalid pattern is reported once, then silently
returns `nil` on subsequent calls.

### rune.regex.compile

```lua
rune.regex.compile(pattern) -> re | nil, err
```

- `pattern` (string) — Go regexp.

Returns a compiled regex object, or `nil` plus an error message. Call
`re:match(text)` to match — useful in hot paths where you'd rather
hold the compiled object than go through the cache:

```lua
local re = assert(rune.regex.compile("^(\\w+) says"))
local m = re:match("Bob says hi")  -- {"Bob says", "Bob"}
```

Unlike `rune.regex.match`, `re:match` returns the **full match at
index 1** with capture groups from index 2, or `nil` on no match.

## Validation

`rune.regex.validate(pattern)` checks a pattern without matching:
`true` for a valid pattern, `nil` plus the compile error otherwise.
Use it when accepting patterns from somewhere you don't control.

**Related:** [Triggers guide](/scripting/triggers/) ·
[rune.trigger](/reference/api/trigger/) ·
[rune.alias](/reference/api/alias/)
