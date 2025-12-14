# Rune Lua API Reference

## Quick Reference

### Core
| Function | Description |
|----------|-------------|
| `rune.send(text)` | Send command (processes aliases) |
| `rune.send_raw(text)` | Send directly to server (bypasses aliases) |
| `rune.echo(text)` | Print to local display |
| `rune.connect(addr)` | Connect to server |
| `rune.disconnect()` | Disconnect from server |
| `rune.load(path)` | Load a Lua script |
| `rune.reload()` | Reload all scripts |
| `rune.quit()` | Exit the client |

### Aliases
| Function | Description |
|----------|-------------|
| `rune.alias.exact(cmd, action, opts?)` | Match command word exactly |
| `rune.alias.regex(pattern, action, opts?)` | Match with Go regexp |
| `rune.alias.disable(name)` | Disable by name |
| `rune.alias.enable(name)` | Enable by name |
| `rune.alias.remove(name)` | Remove by name |
| `rune.alias.list()` | List all aliases |
| `rune.alias.clear()` | Remove all |
| `rune.alias.count()` | Count total |
| `rune.alias.remove_group(group)` | Remove all in group |

### Triggers
| Function | Description |
|----------|-------------|
| `rune.trigger.exact(line, action, opts?)` | Match whole line exactly |
| `rune.trigger.starts(prefix, action, opts?)` | Match line prefix |
| `rune.trigger.contains(substr, action, opts?)` | Match substring |
| `rune.trigger.regex(pattern, action, opts?)` | Match with Go regexp |
| `rune.trigger.disable(name)` | Disable by name |
| `rune.trigger.enable(name)` | Enable by name |
| `rune.trigger.remove(name)` | Remove by name |
| `rune.trigger.list()` | List all triggers |
| `rune.trigger.clear()` | Remove all |
| `rune.trigger.count()` | Count total |
| `rune.trigger.remove_group(group)` | Remove all in group |

### Timers
| Function | Description |
|----------|-------------|
| `rune.timer.after(secs, action, opts?)` | One-shot timer |
| `rune.timer.every(secs, action, opts?)` | Repeating timer |
| `rune.timer.disable(name)` | Disable by name |
| `rune.timer.enable(name)` | Enable by name |
| `rune.timer.remove(name)` | Remove by name |
| `rune.timer.cancel(name)` | Same as remove |
| `rune.timer.list()` | List all timers |
| `rune.timer.clear()` | Cancel all |
| `rune.timer.count()` | Count total |
| `rune.timer.remove_group(group)` | Cancel all in group |

### Hooks
| Function | Description |
|----------|-------------|
| `rune.hooks.on(event, handler, opts?)` | Attach event handler |
| `rune.hooks.disable(name)` | Disable by name |
| `rune.hooks.enable(name)` | Enable by name |
| `rune.hooks.remove(name)` | Remove by name |
| `rune.hooks.list()` | List all handlers |
| `rune.hooks.clear(event?)` | Clear handlers |
| `rune.hooks.remove_group(group)` | Remove all in group |

### Groups
| Function | Description |
|----------|-------------|
| `rune.group.disable(name)` | Disable group (master switch) |
| `rune.group.enable(name)` | Enable group |
| `rune.group.is_enabled(name)` | Check if enabled |
| `rune.group.list()` | List all groups |

### Regex
| Function | Description |
|----------|-------------|
| `rune.regex.match(pattern, text)` | Match and return captures |
| `rune.regex.compile(pattern)` | Compile for reuse |

### UI Layout
| Function | Description |
|----------|-------------|
| `rune.ui.layout(config)` | Configure top/bottom dock layout |
| `rune.ui.bar(name, render_fn)` | Register a bar renderer (pull-based) |

### Panes
| Function | Description |
|----------|-------------|
| `rune.pane.create(name)` | Create a named pane (push-based) |
| `rune.pane.write(name, text)` | Append line to pane |
| `rune.pane.toggle(name)` | Show/hide pane |
| `rune.pane.clear(name)` | Clear pane contents |

### Picker
| Function | Description |
|----------|-------------|
| `rune.ui.picker.show(opts)` | Show fuzzy-filter selection panel |

### Keybindings
| Function | Description |
|----------|-------------|
| `rune.bind(key, callback)` | Register a keybinding |
| `rune.unbind(key)` | Remove a keybinding |

### Input
| Function | Description |
|----------|-------------|
| `rune.input.get()` | Get current input field text |
| `rune.input.set(text)` | Set input field text |

### History
| Function | Description |
|----------|-------------|
| `rune.history.get()` | Get command history array |

---

## Core Functions

The fundamental functions for interacting with the MUD server and client.

### Sending Commands

```lua
rune.send(text)      -- Process through aliases, then send to server
rune.send_raw(text)  -- Bypass aliases, send directly to socket
```

### Output

```lua
rune.echo(text)      -- Print to local display (not sent to server)
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

## Scripting Elements

Aliases, triggers, timers, and hooks share common patterns for creation and management.

### Common Patterns

#### Handles

All creation functions return a handle with these methods:

```lua
local h = rune.alias.exact("foo", "bar", {name = "my_alias", group = "stuff"})

h:disable()   -- Disable this item
h:enable()    -- Enable this item
h:remove()    -- Remove this item
h:name()      -- Get the name (nil if unnamed)
h:group()     -- Get the group (nil if ungrouped)
```

Methods are chainable:

```lua
rune.trigger.contains("spam", nil, {gag = true}):disable()
```

#### Options

All creation functions accept an optional `opts` table:

| Option | Type | Default | Applies To | Description |
|--------|------|---------|------------|-------------|
| `name` | string | nil | All | Unique ID for management. Enables upsert. |
| `group` | string | nil | All | Group membership for batch operations. |
| `once` | bool | false | Alias, Trigger | Auto-remove after first match. |
| `priority` | number | 50 | Alias, Trigger, Hook | Execution order (lower = first). |
| `gag` | bool | false | Trigger | Hide matching line from display. |
| `raw` | bool | false | Trigger | Match against raw line (with ANSI codes). |

#### Actions

Actions can be strings or functions:

**String actions** are sent as commands:
```lua
rune.alias.exact("n", "north")
rune.trigger.contains("hungry", "eat bread")
rune.timer.every(60, "save")
```

**Function actions** receive context:
```lua
rune.alias.exact("heal", function(args, ctx)
    rune.send("cast heal " .. (args or "self"))
end)

rune.trigger.regex("^(\\w+) attacks", function(matches, ctx)
    rune.send("defend " .. matches[1])
end)

rune.timer.every(10, function(ctx)
    if should_stop then ctx:remove() end
end)
```

#### Context Objects

Function actions receive a context object (`ctx`):

| Field | Description |
|-------|-------------|
| `ctx.name` | Item name (if set) |
| `ctx.group` | Item group (if set) |
| `ctx.type` | "alias", "trigger", "timer", or "hook" |
| `ctx.line` | Original input/output line |
| `ctx.args` | Args string (exact aliases only) |
| `ctx.matches` | Capture array (regex only) |
| `ctx:remove()` | Remove this item (timers) |

---

### Aliases

Aliases match user input and transform or expand it.

#### Creation

```lua
rune.alias.exact(command, action, opts?)  -- Match command word (literal)
rune.alias.regex(pattern, action, opts?)  -- Match with Go regexp
```

#### Exact Aliases

Matches the first word literally. Trailing args are appended to string expansions:

```lua
rune.alias.exact("n", "north")       -- "n" → "north"
rune.alias.exact("g", "get")         -- "g sword" → "get sword"

rune.alias.exact("heal", function(args, ctx)
    -- args = "bob" if input was "heal bob"
    rune.send("cast heal " .. (args or "self"))
end)
```

#### Regex Aliases

Matches the full input line with Go regexp. Use when you need captures or complex structure.

**String actions** use `%1`, `%2`, etc. for capture substitution:

```lua
-- "cmd bob attack" → "command private bob to attack"
rune.alias.regex("^cmd\\s+(\\w+)\\s+(.+)", "command private %1 to %2")
```

**Function actions** receive a `matches` array:

```lua
-- "give 5 coins to bob" → sends "give coins bob" 5 times
rune.alias.regex("^give\\s+(\\d+)\\s+(\\w+)\\s+to\\s+(\\w+)", function(matches, ctx)
    local count = tonumber(matches[1])  -- "5"
    local item = matches[2]              -- "coins"
    local target = matches[3]            -- "bob"
    for i = 1, count do
        rune.send("give " .. item .. " " .. target)
    end
end)
```

---

### Triggers

Triggers match server output and execute actions.

#### Creation

```lua
rune.trigger.exact(line, action, opts?)      -- Whole line matches exactly
rune.trigger.starts(prefix, action, opts?)   -- Line starts with prefix
rune.trigger.contains(substr, action, opts?) -- Line contains substring
rune.trigger.regex(pattern, action, opts?)   -- Go regexp match
```

#### Match Modes

| Mode | Matching | Captures |
|------|----------|----------|
| `exact` | Whole line must match exactly | Empty table |
| `starts` | Line begins with prefix | Empty table |
| `contains` | Line contains substring | Empty table |
| `regex` | Go regexp against line | Captured groups |

#### Return Values

Function triggers can control line display:

| Return | Effect |
|--------|--------|
| `nil` | Line passes through unchanged |
| `false` | Gag the line (hide from display) |
| `string` | Replace line with returned string |

```lua
rune.trigger.contains("spam", function()
    return false  -- Gag
end)

rune.trigger.regex("^(.+)$", function(matches)
    return "[MOD] " .. matches[1]  -- Modify
end)
```

#### Line Object

Trigger functions receive a Line object in `ctx.line`:

```lua
rune.trigger.regex("pattern", function(matches, ctx)
    local raw = ctx.line:raw()     -- With ANSI codes
    local clean = ctx.line:clean() -- Without ANSI codes
end)
```

---

### Timers

Timers execute actions after a delay or repeatedly.

#### Creation

```lua
rune.timer.after(seconds, action, opts?)  -- One-shot (fires once)
rune.timer.every(seconds, action, opts?)  -- Repeating (fires every interval)
```

#### Self-Cancellation

Timers can cancel themselves from within:

```lua
rune.timer.every(10, function(ctx)
    if done then
        ctx:remove()  -- Stop this timer
    end
end)
```

#### Handle Methods

Timer handles support both `:remove()` and `:cancel()` (equivalent):

```lua
local h = rune.timer.every(5, "tick")
h:cancel()   -- Stop the timer
h:remove()   -- Same as cancel
h:disable()  -- Pause without removing
h:enable()   -- Resume
```

---

### Hooks

Hooks attach handlers to system events with priority ordering.

#### Creation

```lua
rune.hooks.on(event, handler, opts?)
```

#### Events

**Data Flow Events** (support return values):

| Event | Handler | Description |
|-------|---------|-------------|
| `"input"` | `function(text)` | User input before processing |
| `"output"` | `function(line)` | Server output (line object) |
| `"prompt"` | `function(line)` | Server prompt (line object) |

**System Events** (notifications only):

| Event | Handler | Description |
|-------|---------|-------------|
| `"connecting"` | `function(addr)` | Before connection |
| `"connected"` | `function(addr)` | After connection |
| `"disconnecting"` | `function()` | Before disconnect |
| `"disconnected"` | `function()` | After disconnect |
| `"reloading"` | `function()` | Before reload |
| `"reloaded"` | `function()` | After reload |
| `"loaded"` | `function(path)` | After script load |
| `"error"` | `function(msg)` | On system error |

#### Return Values (Data Flow Only)

| Return | Effect |
|--------|--------|
| `false` | Gag/stop |
| `nil` | Pass through |
| `string` | Replace text |

```lua
rune.hooks.on("output", function(line)
    if line:clean():match("tells you:") then
        return "\027[36m" .. line:raw() .. "\027[0m"  -- Cyan
    end
end, {priority = 50})
```

---

## Groups

Groups provide batch enable/disable over aliases, triggers, timers, and hooks.

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

-- Disable entire group
rune.group.disable("combat")  -- All combat items stop firing

-- Re-enable group
rune.group.enable("combat")   -- Items resume (individual states preserved)
```

### API

```lua
rune.group.disable(name)     -- Disable group (master switch off)
rune.group.enable(name)      -- Enable group (master switch on)
rune.group.is_enabled(name)  -- Check if enabled (default: true)
rune.group.list()            -- List all groups with states
```

### Removing by Group

Each module handles its own removal:

```lua
rune.alias.remove_group("combat")
rune.trigger.remove_group("combat")
rune.timer.remove_group("combat")
rune.hooks.remove_group("combat")
```

---

## UI System

The UI consists of a main output viewport with configurable **docks** at the top and bottom. Docks contain **bars** and **panes** that display status information, chat logs, or other auxiliary content.

### Architecture

```
┌─────────────────────────────────────────┐
│  Top Dock (panes, bars)                 │
├─────────────────────────────────────────┤
│                                         │
│           Main Output                   │
│           (server text)                 │
│                                         │
├─────────────────────────────────────────┤
│  Bottom Dock (input, status bar)        │
└─────────────────────────────────────────┘
```

**Bars** are **pull-based**: the system calls your render function periodically (every 250ms) to get current content. Use bars for dynamic status displays that reflect current state.

**Panes** are **push-based**: you write lines to a buffer, and the pane displays them. Use panes for scrollable logs like chat, combat, or debug output.

### Layout Configuration

```lua
rune.ui.layout({
    top = { ... },      -- Components in top dock
    bottom = { ... }    -- Components in bottom dock
})
```

Layout entries can be:
- **Strings**: Component name (e.g., `"input"`, `"status"`, pane name)
- **Tables**: Component with options (e.g., `{name = "tells", height = 10}`)

Built-in components:
- `"input"` - The command input line
- `"status"` - The default status bar

```lua
-- Default layout
rune.ui.layout({
    bottom = { "input", "status" }
})

-- Add a chat pane to top dock
rune.ui.layout({
    top = { {name = "tells", height = 8} },
    bottom = { "input", "status" }
})
```

### Bars (Pull-Based)

Bars display dynamic status information. Register a render function that returns content:

```lua
rune.ui.bar(name, render_fn)
```

The render function receives the terminal width and returns either:
- A string (displayed left-aligned)
- A table with `{left, center, right}` fields

```lua
-- Status bar showing connection and scroll state
rune.ui.bar("status", function(width)
    local state = rune.state

    local left
    if state.connected then
        left = "\027[32m●\027[0m " .. state.address
    else
        left = "\027[90m● Disconnected\027[0m"
    end

    local right = state.scroll_mode == "scrolled"
        and "SCROLL (" .. state.scroll_lines .. " new)"
        or "LIVE"

    return { left = left, right = right }
end)
```

**Available state** (`rune.state`):
- `connected` - Boolean connection status
- `address` - Server address (e.g., "mud.example.com:23")
- `scroll_mode` - "live" or "scrolled"
- `scroll_lines` - New lines while scrolled
- `width`, `height` - Terminal dimensions

### Panes (Push-Based)

Panes are scrollable text buffers for logs and chat. Write lines to them as events happen:

```lua
rune.pane.create(name)      -- Create a named pane (optional, auto-creates on write)
rune.pane.write(name, text) -- Append a line to the pane
rune.pane.toggle(name)      -- Show/hide the pane
rune.pane.clear(name)       -- Clear all contents
```

```lua
-- Log combat to a pane
rune.trigger.regex("^You hit (.+) for (\\d+)", function(matches)
    rune.pane.write("combat", "Hit " .. matches[1] .. " for " .. matches[2])
end)

-- Toggle with a keybind
rune.bind("f5", function()
    rune.pane.toggle("combat")
end)
```

Pane buffer is limited to 1000 lines (auto-trims to 500 when exceeded)

---

## Picker

The picker is a fuzzy-filtering selection panel for choosing from a list of items.

### API

```lua
rune.ui.picker.show({
    title = "Title",                    -- Optional header (modal mode only)
    items = {...},                      -- Array of items (see formats below)
    on_select = function(value) end,    -- Callback when item selected
    mode = "modal",                     -- "modal" (default) or "inline"
    match_description = false           -- Include description in fuzzy matching
})
```

### Item Formats

**Simple strings** (text and value are the same):
```lua
items = {"north", "south", "east", "west"}
```

**Tables with fields**:
```lua
items = {
    {text = "go north", desc = "Move to the forest", value = "north"},
    {text = "go south", desc = "Move to the town", value = "south"},
}
```

### Modes

**Modal mode** (default): Captures all keyboard input. Has its own search field. Used for focused selection dialogs.

```lua
-- Search command history
rune.bind("ctrl+r", function()
    local history = rune.history.get()
    local items = {}
    for i = #history, 1, -1 do
        table.insert(items, history[i])
    end
    rune.ui.picker.show({
        title = "History",
        items = items,
        on_select = function(val)
            rune.input.set(val)
        end
    })
end)
```

**Inline mode**: Filters based on current input field content as you type. Used for autocomplete-style selection.

```lua
-- Slash command autocomplete
rune.bind("/", function()
    rune.input.set("/")
    local cmds = rune.command.list()
    local items = {}
    for _, c in ipairs(cmds) do
        table.insert(items, {
            text = "/" .. c.name,
            desc = c.description,
            value = "/" .. c.name
        })
    end
    rune.ui.picker.show({
        items = items,
        mode = "inline",
        match_description = true,
        on_select = function(val) end
    })
end)
```

### Picker Navigation

| Key | Action |
|-----|--------|
| `up` / `down` | Navigate selection |
| `enter` / `tab` | Accept selection |
| `esc` | Cancel |
| Typing | Filter items |

---

## Keybindings

Custom keyboard shortcuts that trigger Lua callbacks.

### API

```lua
rune.bind(key, callback)   -- Register a keybinding
rune.unbind(key)           -- Remove a keybinding
```

### Key Formats

| Format | Examples |
|--------|----------|
| Single character | `"j"`, `"/"`, `"."` |
| Ctrl combinations | `"ctrl+r"`, `"ctrl+t"`, `"ctrl+a"` |
| Function keys | `"f1"` through `"f12"` |
| Navigation | `"up"`, `"down"` |

### Default Bindings

| Key | Action |
|-----|--------|
| `ctrl+r` | History search (modal picker) |
| `ctrl+t` | Alias search (modal picker) |
| `/` | Slash command autocomplete (inline picker) |

### Built-in Navigation

These keys are always active and cannot be rebound:

| Key | Action |
|-----|--------|
| `enter` | Submit input |
| `ctrl+c` | Quit (empty input) / Cancel / Clear |
| `esc` | Cancel / Clear input |
| `ctrl+u` | Clear entire input line |
| `ctrl+w` | Delete previous word |
| `page up` / `page down` | Scroll output viewport |
| `home` / `end` | Jump to top/bottom of output |

### Example

```lua
-- Quick directional movement
rune.bind("f1", function() rune.send("north") end)
rune.bind("f2", function() rune.send("south") end)
rune.bind("f3", function() rune.send("east") end)
rune.bind("f4", function() rune.send("west") end)

-- Toggle a pane
rune.bind("f5", function() rune.pane.toggle("combat") end)
```

---

## Input API

Control the input field programmatically.

```lua
rune.input.get()      -- Get current input text
rune.input.set(text)  -- Set input text (used by picker callbacks, etc.)
```

---

## History API

Access command history.

```lua
rune.history.get()    -- Returns array of past commands (oldest first)
```

---

## Regex API

Rune uses Go's regexp syntax (not Lua patterns).

### Functions

```lua
rune.regex.match(pattern, text)  -- Returns captures array or nil
rune.regex.compile(pattern)      -- Returns compiled regex or nil, error
```

### Match

Returns captured groups (excluding full match), or `nil` if no match:

```lua
local caps = rune.regex.match("^(\\w+)\\s+(\\d+)", "foo 42")
-- caps = {"foo", "42"}
```

### Compiled Regex

For performance, compile once and reuse:

```lua
local re = rune.regex.compile("^(\\w+)")
local matches = re:match("hello world")  -- {"hello"}
```

### Pattern Notes

- Use `\\` to escape in Lua strings: `"\\d+"` matches digits
- Go regexp, not Lua patterns: `\\w`, `\\s`, `\\d` work; `%w`, `%s` don't
- Captures use parentheses: `(\\w+)`
- Non-capturing groups: `(?:\\w+)`

---

## Examples

### Combat System

```lua
rune.alias.exact("combat", function()
    rune.group.enable("combat")
    rune.echo("[Combat mode ON]")
end)

rune.alias.exact("peace", function()
    rune.group.disable("combat")
    rune.echo("[Combat mode OFF]")
end)

rune.trigger.regex("^HP: (\\d+)/", function(matches)
    if tonumber(matches[1]) < 50 then
        rune.send("flee")
    end
end, {group = "combat", name = "auto_flee"})

rune.timer.every(10, function()
    rune.send("check hp")
end, {group = "combat", name = "hp_check"})
```

### Repeat Command

```lua
rune.alias.regex("^#(\\d+)\\s+(.+)", function(matches)
    local count = tonumber(matches[1])
    local cmd = matches[2]
    for i = 1, count do
        rune.send(cmd)
    end
end, {name = "repeat_cmd"})

-- Usage: #5 north
```

### Gag Spam

```lua
local spam = {"^\\[Advertisement\\]", "^\\[OOC\\]", "You are too tired"}

for i, pat in ipairs(spam) do
    rune.trigger.regex(pat, nil, {name = "gag_" .. i, gag = true})
end
```

### Wait for Player

```lua
rune.alias.exact("waitfor", function(args)
    rune.trigger.contains(args .. " has arrived", function()
        rune.send("wave " .. args)
        rune.echo("[" .. args .. " arrived!]")
    end, {once = true})
    rune.echo("[Waiting for " .. args .. "...]")
end)
```

### Highlight Tells

```lua
rune.hooks.on("output", function(line)
    if line:clean():match("tells you:") then
        return "\027[36m" .. line:raw() .. "\027[0m"
    end
end, {name = "highlight_tells", priority = 50})
```

### Tell Window (Quake-Style Dropdown)

A chat pane in the top dock that captures tells and can be toggled with backtick, like the Quake console:

```lua
-- Configure layout with tells pane in top dock
rune.ui.layout({
    top = { {name = "tells", height = 8} },
    bottom = { "input", "status" }
})

-- Capture tells and channel chat to the pane
rune.trigger.regex("(\\w+) tells you:", function(matches, ctx)
    rune.pane.write("tells", ctx.line:clean())
end, {name = "capture_tells"})

rune.trigger.regex("^\\[(\\w+)\\]", function(matches, ctx)
    rune.pane.write("tells", ctx.line:clean())
end, {name = "capture_channels"})

-- Toggle with backtick (Quake-style)
rune.bind("`", function()
    rune.pane.toggle("tells")
end)

-- Clear chat log
rune.alias.exact("clearchat", function()
    rune.pane.clear("tells")
    rune.echo("[Chat cleared]")
end)
```

The pane starts hidden and drops down when you press backtick, showing the last 8 lines of tells and channel chat. Press backtick again to hide it.
