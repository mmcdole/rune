---
title: Logging
description: Session transcripts that read like your screen, with a logging policy you can rewrite.
---

One command starts a transcript:

```txt
/log start             -- ~/.config/rune/logs/<timestamp>.log
/log start quest.log   -- or name it
/log status
/log stop
```

The log is ANSI-stripped and mirrors what you saw: server lines after
triggers ran (rewrites logged as rewritten, gagged lines omitted) plus the
local echo of what you typed. Prompts are skipped. Passwords stay out,
because the echo doesn't fire while the server hides input.

An active log survives `/reload` (the file handle is owned by the client,
not the Lua VM) and closes cleanly on exit.

## From Lua

```lua
rune.log.start(path?)   -- returns the resolved path, or nil + error
rune.log.stop()         -- returns true if a log was open
rune.log.status()       -- active path or nil
rune.log.write(text)    -- append a line directly (no-op when not logging)
```

## Changing the policy

What gets written is two hooks, named `log-output` and `log-echo` at
priority 200. To add timestamps, replace one:

```lua
rune.hooks.disable("log-output")
rune.hooks.on("output", function(line)
    rune.log.write(os.date("[%H:%M:%S] ") .. line:clean())
end, { priority = 200 })
```

To log every raw line including gagged ones, register at a priority below
100, before the trigger handler runs.

To auto-log every session:

```lua
rune.hooks.on("connected", function(addr)
    if not rune.log.status() then
        rune.log.start()
    end
end)
```

**Related:** [rune.log reference](/reference/api/log/),
[Hooks & Events](/scripting/hooks/),
[Triggers](/scripting/triggers/)
