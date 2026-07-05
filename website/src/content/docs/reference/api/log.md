---
title: rune.log
description: Full signatures for session logging — start, stop, status, direct writes, and the logging policy.
---

Log the session to a file. For a task-oriented introduction, see
[Logging](/scripting/logging/).

## Quick reference

```lua
rune.log.start(path?, opts?)  -- start logging; returns resolved path, or nil + err
rune.log.stop()               -- stop; true if a log was open
rune.log.status()             -- the active log path, or nil
rune.log.write(text)          -- append a line directly (no-op while not logging)
```

`start` defaults the path to `<config_dir>/logs/<timestamp>.log` and
stamps a header line; `stop` stamps a footer. `opts` takes
`{ raw = true }` to keep ANSI codes in the log instead of stripping
them — a color-faithful transcript, viewable with `less -R`. The file
handle is Go-owned: an active log (raw mode included) survives
`/reload` and is closed cleanly on exit. `/log start [raw] [file]`,
`/log stop`, and `/log status` drive the same functions from the
input line; `/log status` shows `(raw)` when the mode is on.

## The logging policy

What gets written is Lua policy, carried by two priority-200 hooks
named `log-output` and `log-echo`: server output after trigger
processing (rewrites are logged as rewritten, gagged lines are not
logged) and the local echo of typed input. Plain mode strips ANSI so
the log reads like the screen; raw mode keeps the codes. Prompts and
client messages (`rune.echo`) are not logged.

:::note
The echo hook does not fire while the server suppresses echo, so
passwords stay out of logs.
:::

For a different policy, disable the default hooks and write your own
against `rune.log.write`:

```lua
rune.hooks.disable("log-output")
rune.hooks.on("output", function(line)
    rune.log.write(os.date("[%H:%M:%S] ") .. line:clean())
end, { priority = 200 })
```

**Related:** [Logging guide](/scripting/logging/) ·
[rune.hooks](/reference/api/hooks/) ·
[rune.trigger](/reference/api/trigger/)
