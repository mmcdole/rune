---
title: rune.style
description: ANSI color and attribute helpers — the idiomatic way to style echoes, rewrites, and bars.
---

Wrap text in ANSI codes without writing escape sequences yourself.
This is the styling used for colored [trigger rewrites](/scripting/triggers/)
and [bar](/interface/bars/) content.

## Quick reference

```lua
rune.style.<color>(s)      -- red green yellow blue magenta cyan white gray
rune.style.<attribute>(s)  -- bold dim inverse
```

Every helper has the same shape: it takes a value, `tostring`s it,
and returns it wrapped in the escape code plus a trailing reset — so
styled fragments never bleed into the text after them.

| Kind | Helpers |
|---|---|
| Colors | `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, `gray` |
| Attributes | `bold`, `dim`, `inverse` |

## Composing

Helpers nest and concatenate like any string functions:

```lua
rune.echo(rune.style.red("[Alert]") .. " Low HP!")
rune.echo(rune.style.bold(rune.style.cyan("nested works too")))
```

Because each helper appends a reset, the innermost reset also ends the
outer style — put the attribute on the outside (as above) or restyle
each fragment separately rather than expecting an outer color to
continue past a nested piece.

Use it anywhere text reaches the screen: `rune.echo` messages, string
returns from `"output"` handlers, and [trigger](/scripting/triggers/)
rewrites:

```lua
rune.trigger.contains("tells you:", function(_, ctx)
    return rune.style.cyan(ctx.line:clean())
end)
```

**Related:** [Triggers guide](/scripting/triggers/) ·
[Bars](/interface/bars/) · [State & Lines](/reference/api/state-lines/) ·
[Core](/reference/api/core/)
