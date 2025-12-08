# Rune Lua API Reference

## Overview

Rune exposes its functionality through the `rune` namespace. The API is designed around explicit matching modes and consistent handle-based management.

**Key Concepts:**
- **Explicit modes**: Always specify `exact`, `starts`, `contains`, or `regex` - no magic detection
- **Handles**: Creation functions return handles with `:disable()`, `:enable()`, `:remove()` methods
- **Groups**: Two-level enable/disable system for batch operations
- **Go Regexp**: Regex matching uses Go's regexp syntax (not Lua patterns)
- **Upsert by name**: Named items replace existing items with the same name

---

## Aliases

Aliases match user input and transform or expand it.

### API

```lua
-- Literal matching
rune.alias.exact(key, action, opts?)    -- Match command word exactly (literal)

-- Regex matching (Go regexp syntax)
rune.alias.regex(pattern, action, opts?)  -- Go regexp on full input line
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `key` | string | Command word to match (for `exact`, literal) |
| `pattern` | string | Go regexp pattern (for `regex`) |
| `action` | string or function | Expansion or handler |
| `opts` | table | Optional settings (see Options) |

### Action Types

**String action**: Expansion text. For regex aliases, `%1`, `%2`, etc. are substituted from captures.

```lua
-- Exact: appends any trailing args automatically
rune.alias.exact("n", "north")  -- "n foo" → "north foo"

-- Regex: use captures for substitution
rune.alias.regex("^go\\s+(\\w+)", "walk %1")  -- "go north" → "walk north"
```

**Function action**: Different signatures for exact vs regex.

```lua
-- Exact: function(args, ctx) - args is string after command word
rune.alias.exact("heal", function(args)
    local target = args or "self"  -- args = "bob" if input was "heal bob"
    rune.send("cast heal " .. target)
end)

-- Regex: function(matches, ctx) - matches is array of captures
rune.alias.regex("^x(\\d+)\\s+(.+)", function(matches)
    local count = tonumber(matches[1])
    local cmd = matches[2]
    for i = 1, count do
        rune.send(cmd)
    end
end)
```

### Exact vs Regex

Use `exact` when matching a command word (literal):
```lua
rune.alias.exact("flee", "escape")  -- Matches "flee" or "flee north"
```

Use `regex` when you need captures or complex matching:
```lua
rune.alias.regex("^heal\\s+(\\w+)\\s+(\\d+)", function(matches)
    local target, amount = matches[1], matches[2]
    rune.send("cast heal " .. target .. " " .. amount)
end)
```

### Context Object

The optional `ctx` parameter provides metadata:

```lua
rune.alias.exact("info", function(args, ctx)
    -- ctx.line  = original input ("info target")
    -- ctx.name  = alias name (if set)
    -- ctx.group = alias group (if set)
    -- ctx.type  = "alias"
    -- ctx.args  = same as args parameter
end)

rune.alias.regex("^x(\\d+)", function(matches, ctx)
    -- ctx.line    = original input
    -- ctx.name    = alias name (if set)
    -- ctx.group   = alias group (if set)
    -- ctx.type    = "alias"
    -- ctx.matches = same as matches parameter
end)
```

### Management Functions

```lua
rune.alias.disable(name)     -- Disable by name, returns true/false
rune.alias.enable(name)      -- Enable by name, returns true/false
rune.alias.remove(name)      -- Remove by name, returns true/false
rune.alias.list()            -- Returns array of alias info
rune.alias.clear()            -- Remove all aliases
rune.alias.count()            -- Return total count
rune.alias.remove_group(name) -- Remove all aliases in group
```

### List Return Format

```lua
{
    {match = "n", value = "north", mode = "exact", name = nil, enabled = true, group = nil, once = false},
    {match = "^go\\s+(\\w+)", value = "walk %1", mode = "regex", name = "go_alias", enabled = true, group = "movement", once = false},
}
```

---

## Triggers

Triggers match server output and execute actions.

### API

```lua
-- Literal matching (no special characters)
rune.trigger.exact(line, action, opts?)       -- Whole line must match exactly
rune.trigger.starts(prefix, action, opts?)    -- Line starts with prefix
rune.trigger.contains(substr, action, opts?)  -- Line contains substring

-- Regex matching (Go regexp syntax)
rune.trigger.regex(pattern, action, opts?)    -- Go regexp match with captures
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `line` | string | Exact line to match (literal) |
| `prefix` | string | Line prefix to match (literal) |
| `substr` | string | Substring to find (literal) |
| `pattern` | string | Go regexp pattern |
| `action` | string or function | Command or handler |
| `opts` | table | Optional settings (see Options) |

### Action Types

**String action**: Sent as command. For regex triggers, `%1`, `%2`, etc. are substituted from captures.

```lua
rune.trigger.exact("You are hungry.", "eat bread")
rune.trigger.starts("You feel", "check status")
rune.trigger.contains("tells you", "reply I'm AFK")
rune.trigger.regex("^(\\w+) attacks you", "defend %1")
```

**Function action**: `function(matches, ctx)` or `function()`.

```lua
rune.trigger.regex("^You have (\\d+) gold", function(matches)
    local gold = tonumber(matches[1])
    if gold > 1000 then
        rune.send("deposit " .. (gold - 100))
    end
end)
```

### Context Object

Function actions receive a context object:

```lua
rune.trigger.regex("^(\\w+) says", function(matches, ctx)
    -- ctx.line    = Line object with :raw() and :clean()
    -- ctx.name    = trigger name (if set)
    -- ctx.group   = trigger group (if set)
    -- ctx.type    = "trigger"
    -- ctx.matches = same as matches parameter
end)
```

### Return Values

Functions can return values to control line display:

| Return Value | Effect |
|--------------|--------|
| `nil` (or no return) | Line passes through unchanged |
| `false` | Gag the line (hide from display) |
| `string` | Replace line with returned string |
| `true`, numbers, tables | Ignored (line unchanged) |

```lua
rune.trigger.contains("spam", function()
    return false  -- Gag the line
end)

rune.trigger.regex("^(.+)$", function(matches)
    return "[MOD] " .. matches[1]  -- Replace line text
end)

rune.trigger.contains("secret", function(matches, ctx)
    -- No return = line displays normally
    rune.send("log secret detected")
end)
```

### Trigger Modes

| Mode | Matching | Captures |
|------|----------|----------|
| `exact` | Whole line must match exactly (literal) | None (empty table) |
| `starts` | Line begins with prefix (literal) | None (empty table) |
| `contains` | Line contains substring (literal) | None (empty table) |
| `regex` | Go regexp against line | Captured groups |

### Management Functions

```lua
rune.trigger.disable(name)     -- Disable by name
rune.trigger.enable(name)      -- Enable by name
rune.trigger.remove(name)      -- Remove by name
rune.trigger.list()            -- Returns array of trigger info
rune.trigger.clear()            -- Remove all triggers
rune.trigger.count()            -- Return total count
rune.trigger.remove_group(name) -- Remove all triggers in group
```

### List Return Format

```lua
{
    {match = "You are hungry.", value = "eat bread", mode = "exact", name = nil, enabled = true, group = nil, gag = false, once = false},
    {match = "You feel", value = "check status", mode = "starts", name = nil, enabled = true, group = nil, gag = false, once = false},
    {match = "^(\\w+) attacks", value = "(function)", mode = "regex", name = "combat_trigger", enabled = true, group = "combat", gag = false, once = false},
}
```

---

## Timers

Timers execute actions after a delay or repeatedly at intervals.

### API

```lua
rune.timer.after(seconds, action, opts?)  -- One-shot timer (fires once)
rune.timer.every(seconds, action, opts?)  -- Repeating timer
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `seconds` | number | Delay in seconds (decimals allowed) |
| `action` | string or function | Command or handler |
| `opts` | table | Optional settings (see Options) |

### Action Types

**String action**: Sent as command.

```lua
rune.timer.after(5, "look")           -- Look after 5 seconds
rune.timer.every(60, "save")          -- Save every minute
```

**Function action**: `function(ctx)` or `function()`.

```lua
rune.timer.every(30, function()
    rune.send("check stats")
end)

rune.timer.every(10, function(ctx)
    -- ctx.name   = timer name (if set)
    -- ctx.group  = timer group (if set)
    -- ctx.type   = "timer"
    -- ctx:cancel() = stop this timer

    if should_stop then
        ctx:cancel()
    end
end)
```

### Handle Methods

Timer handles support both `:cancel()` and `:remove()` (they are equivalent):

```lua
local h = rune.timer.every(5, "tick")
h:cancel()   -- Stop the timer
h:remove()   -- Same as cancel (for consistency with aliases/triggers)
h:disable()  -- Pause without removing
h:enable()   -- Resume
```

### Management Functions

```lua
rune.timer.disable(name)     -- Disable by name
rune.timer.enable(name)      -- Enable by name
rune.timer.cancel(name)      -- Cancel by name (stops and removes)
rune.timer.remove(name)      -- Same as cancel (for consistency)
rune.timer.list()            -- Returns array of timer info
rune.timer.clear()            -- Cancel all timers
rune.timer.count()            -- Return total count
rune.timer.remove_group(name) -- Cancel all timers in group
```

### List Return Format

```lua
{
    {seconds = 5.0, mode = "every", value = "save", name = nil, enabled = true, group = nil},
    {seconds = 30.0, mode = "after", value = "(function)", name = "delayed_action", enabled = true, group = "combat"},
}
```

---

## Groups

Groups provide batch operations over aliases, triggers, and timers sharing a group label.

### Two-Level Enable/Disable

Items have two independent enable states:

1. **Group level**: Master switch via `rune.group.disable/enable`
2. **Item level**: Individual state via `handle:disable/enable`

An item fires only if **both** are enabled.

```lua
-- Create items in a group
rune.alias.exact("buff", "cast buff", {group = "combat"})
rune.trigger.starts("Enemy", "attack", {group = "combat"})
rune.timer.every(30, "heal", {group = "combat"})

-- Disable entire group (individual states preserved)
rune.group.disable("combat")

-- Re-enable group (items resume their previous states)
rune.group.enable("combat")
```

### API

```lua
rune.group.disable(name)     -- Suspend all items in group (they won't fire)
rune.group.enable(name)      -- Resume all items in group
rune.group.is_enabled(name)  -- Check if group is enabled (default: true)
```

Note: There is no `list()` function. Groups exist implicitly when items reference them.

### Removing Items by Group

Each module owns its items. Use `remove_group()` at the module level:

```lua
rune.alias.remove_group("combat")   -- Remove all aliases in group
rune.trigger.remove_group("combat") -- Remove all triggers in group
rune.timer.remove_group("combat")   -- Remove all timers in group
rune.hooks.remove_group("combat")   -- Remove all hooks in group
```

### Behavior Notes

- Each item can belong to at most one group (`opts.group` is a single string)
- `disable()` does NOT mutate individual item states - they're preserved for re-enable
- Groups don't need to be pre-declared - they exist implicitly when items reference them

---

## Handle Methods

All creation functions return a handle with these methods:

```lua
local h = rune.alias.exact("foo", "bar", {name = "my_alias", group = "stuff"})

h:disable()   -- Disable this item
h:enable()    -- Enable this item
h:remove()    -- Remove this item (or h:cancel() for timers)
h:name()      -- Get the name (returns nil if unnamed)
h:group()     -- Get the group (returns nil if not in group)
```

Methods are chainable:

```lua
rune.alias.exact("tmp", "temp"):disable()
```

---

## Options Reference

All creation functions accept an optional `opts` table as the last parameter.

### Common Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | nil | Unique ID for management. Enables upsert behavior. |
| `group` | string | nil | Group membership for batch operations. |

### Alias/Trigger Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `once` | boolean | false | Auto-remove after first match. |
| `priority` | number | 50 | Execution order (lower = first). |

### Trigger-Only Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `gag` | boolean | false | Hide matching line from display. |
| `raw` | boolean | false | Match against raw line (with ANSI escape codes). |

### Examples

```lua
-- Named for later management
rune.alias.exact("flee", "escape", {name = "flee_alias"})
rune.alias.remove("flee_alias")

-- One-time trigger that auto-removes
rune.trigger.contains("Welcome!", function()
    rune.send("look")
end, {once = true})

-- High-priority trigger (runs first)
rune.trigger.contains("URGENT", "alert", {priority = 10})

-- Gagging spam
rune.trigger.contains("[Advertisement]", nil, {gag = true})

-- Match ANSI escape codes (e.g., bold red text)
rune.trigger.regex("\\x1b\\[1;31m(\\w+)\\x1b\\[m", function(matches)
    rune.echo("Found highlighted name: " .. matches[1])
end, {raw = true})

-- Group membership
rune.alias.exact("hunt", "track prey", {group = "hunting"})
rune.trigger.contains("prey escapes", "track", {group = "hunting"})
rune.group.disable("hunting")  -- Disable all hunting features
```

---

## Core Functions

### Sending Commands

```lua
rune.send(text)      -- Process through aliases, then send to server
rune.send_raw(text)  -- Bypass aliases, send directly to socket
```

### Output

```lua
rune.echo(text)     -- Print to local display (not sent to server)
```

### Connection

```lua
rune.connect(address)  -- Connect to server (e.g., "example.com:23")
rune.disconnect()      -- Disconnect from server
```

### Scripts

```lua
rune.load(path)   -- Load a Lua script, returns nil on success or error string
rune.reload()     -- Clear state and reload all scripts
```

### Application

```lua
rune.quit()       -- Exit the client
```

### Configuration

```lua
rune.config_dir   -- Path to ~/.config/rune (read-only)
rune.debug        -- Set to true to enable debug output
rune.dbg(msg)     -- Print debug message (only if rune.debug is true)
```

---

## Regex API

Rune uses Go's regexp syntax for pattern matching.

### API

```lua
rune.regex.match(pattern, text)  -- Returns captures array or nil
rune.regex.compile(pattern)      -- Returns compiled regex object or nil, error
```

### Match Function

Returns an array of captured groups (excluding the full match), or `nil` if no match.

```lua
local captures = rune.regex.match("^(\\w+)\\s+(\\d+)", "foo 42")
-- captures = {"foo", "42"}

local caps = rune.regex.match("no match", "text")
-- caps = nil
```

### Compiled Regex

For performance-critical code, compile once and reuse:

```lua
local re = rune.regex.compile("^(\\w+)")
local matches = re:match("hello world")  -- {"hello"}
```

### Pattern Syntax Notes

- Use `\\` to escape in Lua strings: `"\\d+"` matches digits
- Go regexp, not Lua patterns: `\\w`, `\\s`, `\\d` work; `%w`, `%s` don't
- Captures use parentheses: `(\\w+)` captures word
- Non-capturing groups: `(?:\\w+)` matches but doesn't capture

---

## Hooks

The hooks system allows multiple handlers per event with priority ordering.

### API

```lua
rune.hooks.on(event, handler, opts?)  -- Attach handler, returns handle
rune.hooks.remove(name)               -- Remove handler by name
rune.hooks.disable(name)              -- Disable handler by name
rune.hooks.enable(name)               -- Enable handler by name
rune.hooks.list()                     -- List all handlers
rune.hooks.clear()                    -- Clear all handlers for all events
rune.hooks.clear(event)               -- Clear handlers for specific event
rune.hooks.remove_group(group)        -- Remove all handlers in group
```

### Events

**Data Flow Events** (support return values):

| Event | Handler Signature | Description |
|-------|-------------------|-------------|
| `"input"` | `function(text)` | User input before processing |
| `"output"` | `function(line)` | Server output (`line:raw()`, `line:clean()`) |
| `"prompt"` | `function(line)` | Server prompt (`line:raw()`, `line:clean()`) |

**System Events** (notifications, no return values):

| Event | Handler Signature | Description |
|-------|-------------------|-------------|
| `"ready"` | `function()` | System ready |
| `"connecting"` | `function(addr)` | Before connection attempt |
| `"connected"` | `function(addr)` | After successful connection |
| `"disconnecting"` | `function()` | Before disconnect |
| `"disconnected"` | `function()` | After disconnect |
| `"reloading"` | `function()` | Before script reload |
| `"reloaded"` | `function()` | After script reload |
| `"loaded"` | `function(path)` | After loading a script |
| `"error"` | `function(msg)` | On any system error |

### Return Values (Data Flow Events Only)

All handlers use consistent return semantics:

| Return | Effect |
|--------|--------|
| `false` | Gag/stop (hide line or cancel input) |
| `nil` | Pass through unchanged |
| `string` | Replace with modified text |

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | nil | Unique ID for management. Enables upsert behavior. |
| `group` | string | nil | Group membership for batch operations. |
| `priority` | number | 50 | Lower = earlier execution |

### Examples

```lua
-- Data flow: Highlight tells in output
rune.hooks.on("output", function(line)
    local text = line:clean()
    if text:match("tells you:") then
        return "\027[36m" .. line:raw() .. "\027[0m"  -- Cyan highlight
    end
    -- return nil to pass through unchanged
end, {priority = 50})

-- System event: Auto-look on connect
rune.hooks.on("connected", function(addr)
    rune.send("look")
end)
```

---

## Examples

### Combat System

```lua
-- Enable combat mode
rune.alias.exact("combat", function()
    rune.group.enable("combat")
    rune.echo("[Combat mode ON]")
end)

-- Disable combat mode
rune.alias.exact("peace", function()
    rune.group.disable("combat")
    rune.echo("[Combat mode OFF]")
end)

-- Auto-flee at low HP
rune.trigger.regex("^HP: (\\d+)/", function(matches)
    if tonumber(matches[1]) < 50 then
        rune.send("flee")
    end
end, {group = "combat", name = "auto_flee"})

-- Periodic heal check
rune.timer.every(10, function()
    rune.send("check hp")
end, {group = "combat", name = "hp_check"})
```

### Speedwalk

```lua
rune.alias.regex("^#(\\d+)\\s+(.+)", function(matches)
    local count = tonumber(matches[1])
    local cmd = matches[2]
    for i = 1, count do
        rune.send(cmd)
    end
end, {name = "repeat_cmd"})

-- Usage: #5 north (sends "north" 5 times)
```

### Gagging Spam

```lua
local spam_patterns = {
    "^\\[Advertisement\\]",
    "^\\[OOC\\]",
    "You are too tired",
}

for i, pat in ipairs(spam_patterns) do
    rune.trigger.regex(pat, nil, {
        name = "gag_spam_" .. i,
        gag = true
    })
end
```

### Temporary One-Shot Trigger

```lua
rune.alias.exact("waitfor", function(args)
    rune.trigger.contains(args .. " has arrived", function()
        rune.send("wave " .. args)
        rune.echo("[" .. args .. " arrived!]")
    end, {once = true, name = "waitfor_trigger"})
    rune.echo("[Waiting for " .. args .. "...]")
end)
```
