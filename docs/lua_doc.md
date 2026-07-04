# Rune Lua API Reference

## Quick Reference

### Core
| Function | Description |
|----------|-------------|
| `rune.send(text)` | Send command (processes aliases) |
| `rune.send_raw(text)` | Send directly to server (bypasses aliases) |
| `rune.echo(text)` | Print to local display |
| `rune.connect(addr)` | Connect to server (`host:port`, optionally `tls://`) |
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

### Slash Commands
| Function | Description |
|----------|-------------|
| `rune.command.add(name, handler, desc?, opts?)` | Register a `/name` command |
| `rune.command.remove(name)` | Remove by name |
| `rune.command.enable/disable(name)` | Manage by name |
| `rune.command.list()` | List all commands (drives `/help`) |

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
| `rune.regex.match(pattern, text)` | Match and return captures (cached) |
| `rune.regex.validate(pattern)` | Check a pattern: `true` or `nil, err` |
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
| `rune.bind(key, callback, opts?)` | Register a keybinding |
| `rune.unbind(key)` | Remove a keybinding |
| `rune.binds.list()` | List all binds (see `/binds`) |
| `rune.binds.enable/disable/remove(name)` | Manage by name |
| `rune.binds.remove_group(group)` | Remove all in group |

### Bars (management)
| Function | Description |
|----------|-------------|
| `rune.bars.list()` | List all bar renderers (see `/bars`) |
| `rune.bars.enable/disable/remove(name)` | Manage by name |
| `rune.bars.remove_group(group)` | Remove all in group |

### Input
| Function | Description |
|----------|-------------|
| `rune.input.get()` / `rune.input.set(text)` | Input field text |
| `rune.input.get_cursor()` / `rune.input.set_cursor(pos)` | Cursor position |
| `rune.input.open_editor(initial?)` | Edit in `$EDITOR`; returns text, ok |
| `rune.input.word_left/word_right()` | Move cursor by word |
| `rune.input.delete_word()` | Delete word before cursor |

### History
| Function | Description |
|----------|-------------|
| `rune.history.get()` | Get command history array (oldest first) |
| `rune.history.add(cmd)` | Append a command to history |

### Storage
Two tiers - the name tells you the lifetime:

| Function | Description |
|----------|-------------|
| `rune.session.set/get/delete(key, ...)` | String store for **this session**: survives `/reload`, not exit |
| `rune.store.set/get/delete(key, ...)` | **Durable** store (`store.json`): structured values, survives exit |

### Worlds
| Function | Description |
|----------|-------------|
| `rune.world.add(name, address, opts?)` | Save a world bookmark (durable) |
| `rune.world.remove(name)` | Remove a bookmark |
| `rune.world.get(name)` | Entry table (`{address=...}`), or nil |
| `rune.world.list()` | Sorted array of `{name, address}` |

### Logging
| Function | Description |
|----------|-------------|
| `rune.log.start(path?)` | Log the session to a file (default: `<config>/logs/<timestamp>.log`) |
| `rune.log.stop()` | Stop logging |
| `rune.log.status()` | Active log path, or nil |

### GMCP
| Function | Description |
|----------|-------------|
| `rune.gmcp.on(package, handler, opts?)` | Handle a GMCP package (handler gets `data, package`) |
| `rune.gmcp.send(package, value?)` | Send a message (value: JSON-able Lua value) |
| `rune.gmcp.subscribe(package, version?)` | Declare interest (maintains `Core.Supports.Set`) |
| `rune.gmcp.is_enabled()` | GMCP negotiated on this connection? |

### Style
| Function | Description |
|----------|-------------|
| `rune.style.red(s)` etc. | Wrap text in ANSI color codes |

Colors: `red green yellow blue magenta cyan white gray`. Attributes: `bold dim inverse`.

### Lines
| Function | Description |
|----------|-------------|
| `rune.line.new(text)` | Build a line object with `:raw()` and `:clean()` |

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
rune.connect(address)  -- Connect to server (e.g., "example.com:4000")
rune.disconnect()      -- Disconnect from server
```

The address accepts an optional scheme prefix:

```lua
rune.connect("mud.example.com:4000")                -- plain telnet (default)
rune.connect("tls://mud.example.com:4000")          -- TLS, certificate verified
rune.connect("tls+insecure://mud.example.com:4000") -- TLS, no verification
                                                    -- (self-signed certs)
```

The full address (scheme included) is what `rune.state.address` reports
and what the core stores for `/reconnect`.

### Scripts

```lua
rune.load(path)   -- Load a Lua script, returns true, or nil + error message
rune.reload()     -- Clear state and reload all scripts
```

#### Requiring Other Files

When a script is loaded (via `rune.load()` or command line), its directory is temporarily added to Lua's `package.path`. This allows you to `require()` other Lua files relative to the script's location:

```
~/.config/rune/
├── init.lua              -- main script
├── combat.lua            -- require("combat")
└── utils/
    └── helpers.lua       -- require("utils.helpers")
```

```lua
-- In init.lua:
local combat = require("combat")         -- loads combat.lua
local helpers = require("utils.helpers") -- loads utils/helpers.lua
```

Standard Lua `require()` semantics apply: modules are cached after first load, and should return a table of exports.

### Application

```lua
rune.quit()       -- Exit the client
```

### Configuration

```lua
rune.config_dir   -- Path to ~/.config/rune (read-only)
rune.version      -- Client version string (shown by /version)
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
| `string` | Replace the line. Later triggers match against (and receive) the rewritten text. |

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

`every()` uses fixed-interval scheduling: the next firing is scheduled
the moment the previous one fires, regardless of how long the action
takes.

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
| `"echo"` | `function(text)` | Local echo of typed input (plain string). The core handler at priority 100 adds the `"> "` prefix and color; return `false` to hide an echo, a string to restyle it. |

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
| `"gmcp"` | `function(package, data, raw)` | Every GMCP message (catch-all; see GMCP API) |
| `"gmcp_enabled"` | `function()` | GMCP negotiated; the core handler sends `Core.Hello` |

#### Return Values (Data Flow Only)

| Return | Effect |
|--------|--------|
| `false` | Gag/stop |
| `nil` | Pass through |
| `string` | Replace the line. Rewrites chain: the next handler (in priority order) receives the rewritten line. |

```lua
rune.hooks.on("output", function(line)
    if line:clean():match("tells you:") then
        return rune.style.cyan(line:raw())
    end
end, {priority = 50})
```

---

## Groups

Groups provide batch enable/disable over aliases, triggers, timers, hooks, binds, bars, and slash commands.

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

## Slash Commands

Register `/name` commands. Built on the shared registry, so commands get
upsert-by-name, source attribution, and the same failure quarantine as
every other callback: a command that throws three times in a row is
disabled individually (re-enable with `rune.command.enable`), and the
input pipeline keeps working.

```lua
rune.command.add(name, handler, description?, opts?)
-- handler = function(args)  -- args is everything after "/name "
-- opts = {group = "string"}
-- Returns a handle with :enable(), :disable(), :remove()

rune.command.remove(name)         -- Remove by name
rune.command.enable(name)         -- Re-enable (e.g. after quarantine)
rune.command.disable(name)
rune.command.get(name)            -- The raw handler, or nil
rune.command.list()               -- Array of {name, description, enabled,
                                  --   group, source}; drives /help and
                                  --   the "/" picker
```

```lua
rune.command.add("greet", function(args)
    rune.send("say Hello, " .. (args ~= "" and args or "everyone") .. "!")
end, "Greet someone")
```

`/help` is generated from this registry, so user-added commands appear
in it automatically.

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
        left = rune.style.green("●") .. " " .. state.address
    else
        left = rune.style.gray("● Disconnected")
    end

    local right = state.scroll_mode == "scrolled"
        and "SCROLL (" .. state.scroll_lines .. " new)"
        or "LIVE"

    return { left = left, right = right }
end)
```

**Available state** (`rune.state`, read-only - writes raise an error):
- `connected` - Boolean connection status
- `address` - Server address (e.g., "mud.example.com:23")
- `scroll_mode` - "live" or "scrolled"
- `scroll_lines` - New lines while scrolled
- `width`, `height` - Terminal dimensions

A bar renderer that errors on 3 consecutive renders is disabled (with
a notice); re-register the bar to give it a fresh start.

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
    match_description = false,          -- Include description in fuzzy matching
    dismiss_on_space = false            -- Inline mode: close the picker once the
                                        -- input contains a space. For pickers over
                                        -- single-token items (slash commands),
                                        -- where a space means the user has
                                        -- committed and is typing arguments.
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
        dismiss_on_space = true,
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
rune.bind(key, callback, opts?)  -- Register a keybinding (opts: name, group)
rune.unbind(key)                 -- Remove a keybinding
rune.binds.list()                -- Inspect all binds (also: /binds)
```

Binds are registry items like triggers: they support names, groups
(a bind in a disabled group swallows its key), and record the
registering script's file:line.

### Key Formats

| Format | Examples |
|--------|----------|
| Single character | `"j"`, `"/"`, `"."` |
| Ctrl combinations | `"ctrl+r"`, `"ctrl+t"`, `"ctrl+a"` |
| Function keys | `"f1"` through `"f12"` |
| Navigation | `"up"`, `"down"`, `"pageup"`, `"pagedown"`, `"home"`, `"end"`, `"escape"` |

### Key Policy

- `enter` always submits input (owned by the client, not rebindable).
- While a picker is open, `ctrl+c`/`esc` cancel it and other keys are
  captured by the picker.
- **Bound printable keys** (like `"j"`) fire only while the input line
  is empty, so hotkeys don't break typing. Non-printable bound keys
  always fire.
- Everything else - including all the defaults below - is an ordinary
  Lua bind and can be rebound or removed.

### Default Bindings (defined in the Lua core; all rebindable)

| Key | Action |
|-----|--------|
| `ctrl+r` | History search (modal picker) |
| `ctrl+t` | Alias search (modal picker) |
| `/` | Slash command autocomplete (inline picker) |
| `ctrl+c` | Clear input; on empty input, double-tap to quit |
| `escape` | Clear input |
| `ctrl+u` | Clear entire input line |
| `ctrl+w`, `alt+backspace`, `ctrl+backspace` | Delete previous word |
| `up` / `down` | History navigation (prefix-matching) |
| `alt+left` / `alt+right`, `ctrl+left` / `ctrl+right` | Word navigation |
| `tab` / `shift+tab` | Completion cycling |
| `ctrl+e` | Edit input in `$EDITOR` |
| `pageup` / `pagedown` | Scroll output viewport |
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
rune.input.get()             -- Get current input text
rune.input.set(text)         -- Set input text (used by picker callbacks, etc.)
rune.input.get_cursor()      -- Cursor position (clamped to the input length)
rune.input.set_cursor(pos)   -- Move the cursor
rune.input.open_editor(init) -- Open $EDITOR; returns edited_text, ok
rune.input.word_left()       -- Move cursor to previous word boundary
rune.input.word_right()      -- Move cursor to next word boundary
rune.input.delete_word()     -- Delete the word before the cursor
```

---

## History API

Access command history. The buffer is Go-owned, so it survives `/reload`.

```lua
rune.history.get()    -- Returns array of past commands (oldest first)
rune.history.add(cmd) -- Append a command (consecutive duplicates ignored)
```

---

## Storage APIs

Two Go-owned stores with different lifetimes:

| | `rune.session` | `rune.store` |
|---|---|---|
| Survives `/reload` | yes | yes |
| Survives client exit | **no** | **yes** (disk: `<config>/store.json`) |
| Values | strings only | strings, numbers, booleans, JSON-able tables |
| Use for | combat toggles, counters, mid-session scratch | bookmarks, settings, anything durable |

### Session Store

Scoped to this client session: survives `/reload` (the Lua VM is torn
down and rebuilt) but not exit.

```lua
rune.session.set(key, value)  -- Store a string
rune.session.get(key)         -- Returns the string, or nil
rune.session.delete(key)      -- Remove a key
```

### Durable Store

Backed by `store.json` under `rune.config_dir` (pretty-printed, so it
is hand-editable while the client is closed). Writes hit disk
immediately and atomically. Reads are served from memory. A corrupt
file at boot is preserved as `store.json.bak` and reported, never
silently discarded. The core uses it for world bookmarks and the last
connection address (`/reconnect`).

```lua
rune.store.set(key, value)  -- Returns true, or nil + error.
                            -- value: string/number/boolean, or a table
                            -- with all-string keys or array shape 1..n.
                            -- set(key, nil) deletes.
rune.store.get(key)         -- Returns the decoded value, or nil
rune.store.delete(key)      -- Returns true, or nil + error
```

Unstorable values (functions, userdata, mixed-key tables, cycles)
return `nil, err` - nothing is written.

---

## Worlds

Named server bookmarks, stored durably in `rune.store` under the
`"worlds"` key. Managed from the input line with `/world add <name>
<host> <port> [tls|tls+insecure]`, `/world remove <name>`, and
`/world list` (or `/worlds`). `/connect <name>` resolves bookmarks
first, and `/connect` with no arguments opens a picker over them.

```lua
rune.world.add(name, address, opts?) -- Returns true, or nil + error.
                                     -- Names cannot contain spaces, ":" or "/".
                                     -- opts keys are stored verbatim alongside
                                     -- the address (room for future fields).
rune.world.remove(name)              -- Returns true if it existed
rune.world.get(name)                 -- Entry table ({address=...}), or nil
rune.world.list()                    -- Sorted array of {name, address}
```

---

## Logging API

Log the session to a file (`/log start`, `/log stop`, `/log status`
drive this from the input line). The file handle is Go-owned, so an
active log survives `/reload` and is closed cleanly on exit.

```lua
rune.log.start(path?) -- Start logging. Default path:
                      --   <config_dir>/logs/<timestamp>.log
                      -- Returns the resolved path, or nil + error.
rune.log.stop()       -- Stop logging. Returns true if a log was open.
rune.log.status()     -- Active log path, or nil.
rune.log.write(text)  -- Append a line directly (no-op while not logging).
```

What gets written is Lua policy (see `57_log.lua`): server output
lines after trigger processing (rewrites are logged as rewritten,
gagged lines are not logged) and the local echo of typed input, both
ANSI-stripped, so the log reads like the screen. Prompts and client
messages (`rune.echo`) are not logged. The echo hook does not fire
while the server suppresses echo, so passwords stay out of logs.

The default policy lives in two priority-200 hooks named `log-output`
and `log-echo`. For a different policy, disable them and write your
own:

```lua
rune.hooks.disable("log-output")
rune.hooks.on("output", function(line)
    rune.log.write(os.date("[%H:%M:%S] ") .. line:clean())
end, { priority = 200 })
```

---

## GMCP API

GMCP (Generic MUD Communication Protocol) carries structured
out-of-band data - vitals, room info, comm channels - as
`Package.SubPackage <json>` messages over telnet option 201. Go owns
the transport and JSON conversion; everything below is Lua policy in
`59_gmcp.lua`.

### Receiving

```lua
-- Handle a specific package. data is the decoded JSON (table, string,
-- number, boolean - or nil when the message has no body); package is
-- the name as the server sent it. Matching is case-insensitive.
rune.gmcp.on("Char.Vitals", function(data, package)
    hp, maxhp = data.hp, data.maxhp
    rune.ui.refresh_bars()
end, { name = "vitals" })

-- Catch-all: every message, plus the raw JSON text
rune.hooks.on("gmcp", function(package, data, raw)
    rune.dbg(package .. " " .. tostring(raw))
end)
```

Handlers are registry-based: `opts` takes `name`, `group`, `priority`,
handles have `:enable/:disable/:remove`, and a handler that keeps
throwing is quarantined individually. `rune.gmcp.list()` returns them
for `/gmcp`. Malformed JSON from the server is reported through the
`"error"` event and dropped before reaching handlers.

### Sending

```lua
rune.gmcp.send("Char.Skills.Get", { group = "combat" }) -- JSON-able value
rune.gmcp.send("Core.Ping")                             -- bare package
rune.gmcp.send_raw("Core.Hello", '{"client":"Rune"}')   -- pre-encoded (debug)
```

`send` returns `true`, or `nil + error` (not connected, GMCP not
negotiated, unencodable value) and echoes the failure.

### Subscriptions and the Handshake

```lua
rune.gmcp.subscribe("Char")        -- version defaults to 1
rune.gmcp.subscribe("Room", 2)
rune.gmcp.unsubscribe("Char")
rune.gmcp.is_enabled()             -- negotiated on this connection?
```

Subscriptions maintain `Core.Supports.Set` (the full set is re-sent on
every change, per the spec). When the server negotiates GMCP, the
`"gmcp_enabled"` event fires and the core handler (named `gmcp-hello`)
sends `Core.Hello {client, version}` plus the current subscription
set - disable or replace it like any named hook. Subscribe at load
time; the handshake picks it up on connect.

`/gmcp` shows negotiation state, subscriptions, and handlers;
`/gmcp send <package> [json]` sends a raw message for debugging.

---

## Style API

ANSI styling helpers - the idiomatic way to color output in scripts.

```lua
rune.echo(rune.style.red("[Alert]") .. " Low HP!")
rune.echo(rune.style.bold(rune.style.cyan("nested works too")))
```

Colors: `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, `gray`.
Attributes: `bold`, `dim`, `inverse`.

---

## Regex API

Rune uses Go's regexp syntax (not Lua patterns).

### Functions

```lua
rune.regex.match(pattern, text)  -- Returns captures array or nil (cached)
rune.regex.validate(pattern)     -- Returns true, or nil + error message
rune.regex.compile(pattern)      -- Returns compiled regex or nil, error
```

`match` caches compiled patterns (bounded; resets at 512 entries).
Invalid patterns are reported once and then silently ignored -
`rune.trigger.regex`/`rune.alias.regex` validate eagerly at
registration so typos fail loudly instead.

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
        return rune.style.cyan(line:raw())
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
