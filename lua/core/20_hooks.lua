-- Hook Registry System
-- Allows multiple handlers per event with priority ordering.
-- Built on rune.registry (15_registry.lua) for handles, names,
-- groups, and priorities.
--
-- API:
--   rune.hooks.on(event, handler, opts?)  -- Attach handler to event
--
-- Returns a handle with :disable(), :enable(), :remove(), :name(), :group()
--
-- Options:
--   name     = "string"   -- Unique ID for upsert/management
--   group    = "string"   -- Group membership for bulk operations
--   priority = 50         -- Execution order (lower = first, default 50)
--
-- Events (data-flow):
--   "input"        -- User input: (text, context); return false to consume
--   "output"       -- Server output line object (false gags, string rewrites)
--   "prompt"       -- Server prompt line object (false gags, string rewrites)
--   "echo"         -- Local echo of typed input, plain string (false hides,
--                     string rewrites; core handler adds the "> " styling)
-- Events (notifications):
--   "ready"        -- Boot complete
--   "connecting"   -- Dial started
--   "connected"    -- After connection established
--   "disconnecting"-- Disconnect requested
--   "disconnected" -- After disconnection
--   "reloading"    -- Before script reload
--   "reloaded"     -- After script reload
--   "loaded"       -- After a script file loads
--   "error"        -- On system error
--   "input_changed"-- Input line content changed while typing
--   "gmcp"         -- Every GMCP message: (package, data, raw);
--                     catch-all alongside rune.gmcp.on (70_gmcp.lua)
--   "gmcp_enabled" -- GMCP negotiated; the core handler sends Core.Hello

-- Per-event dispatch index, maintained alongside the registry so
-- rune.hooks.call doesn't scan unrelated events on every line.
local by_event = {} -- event -> sorted array of data

local function sort_handlers(handlers)
    table.sort(handlers, function(a, b)
        if a.priority ~= b.priority then
            return a.priority < b.priority
        end
        return a.id < b.id
    end)
end

local registry = rune.registry.new{
    kind = "hook",
    on_add = function(data)
        local handlers = by_event[data.event]
        if not handlers then
            handlers = {}
            by_event[data.event] = handlers
        end
        table.insert(handlers, data)
        sort_handlers(handlers)
    end,
    on_remove = function(data)
        local handlers = by_event[data.event]
        if not handlers then
            return
        end
        for i, entry in ipairs(handlers) do
            if entry == data then
                table.remove(handlers, i)
                break
            end
        end
    end,
}

-- Public API
rune.hooks = {}

-- Attach a handler to an event
-- Returns: Handle with :remove(), :enable(), :disable(), :name(), :group()
function rune.hooks.on(event, handler, opts)
    return registry:add({
        event = event,
        handler = handler,
        source = rune.caller_source(1),
    }, opts)
end

-- Management by name
function rune.hooks.disable(name)
    return registry:disable(name)
end

function rune.hooks.enable(name)
    return registry:enable(name)
end

function rune.hooks.remove(name)
    return registry:remove(name)
end

-- Run a single handler protected. A failing handler is reported and
-- treated as returning nil (pass through), so one broken handler
-- cannot abort the rest of the chain or kill trigger processing.
-- Errors are echoed directly (not re-dispatched through the "error"
-- event) to avoid recursion when an error handler itself fails.
-- Repeated failures disable the handler (see rune.guarded_call).
local function run_handler(entry, ...)
    local label = "Hook " .. entry.event .. " " ..
        (entry.name and ('"' .. entry.name .. '"') or ("#" .. entry.id)) ..
        (entry.source and (" @" .. entry.source) or "")
    local ok, result = rune.guarded_call(label, entry, entry.handler, ...)
    if ok then
        return result
    end
    return nil
end

-- Input contexts describe an immutable submission snapshot. Each handler gets
-- a fresh read-only proxy so even raw mutation cannot change the canonical
-- mode observed by later handlers or by the core router.
local function reject_input_context_write()
    error("input context is read-only", 2)
end

local input_context_metatables = {
    command = {
        __index = { mode = "command" },
        __newindex = reject_input_context_write,
        __metatable = false,
    },
    verbatim = {
        __index = { mode = "verbatim" },
        __newindex = reject_input_context_write,
        __metatable = false,
    },
}

local function input_context(mode)
    return setmetatable({}, input_context_metatables[mode] or input_context_metatables.command)
end

-- Call all handlers for an event
-- For output/prompt: chains modifications (each handler sees the
--   previous handler's rewrite), false gags
-- For echo: like output/prompt, but the argument is a plain string
--   (the text the user typed), not a line object
-- For input: false stops processing
-- For sys events: all handlers run (notifications)
--
-- Return semantics for handlers:
--   return false    -> Stop/Gag
--   return string   -> Replace the line for subsequent handlers
--   return nil      -> Pass through unmodified
function rune.hooks.call(event, ...)
    local live = by_event[event]
    if not live or #live == 0 then
        -- No handlers registered
        if event == "output" or event == "prompt" then
            local line = select(1, ...)
            return line:raw(), true
        elseif event == "echo" then
            return select(1, ...), true
        elseif event == "input" then
            return true
        end
        return
    end

    -- Iterate a snapshot: a handler that adds or removes hooks mutates
    -- the live array, which would skip or double-run its neighbors.
    -- Removals mid-dispatch are still honored via registry:active().
    local handlers = {}
    for i, entry in ipairs(live) do
        handlers[i] = entry
    end

    if event == "output" or event == "prompt" then
        -- Output/prompt receive a Line object (:raw() and :clean()).
        -- True chaining: a handler returning a string replaces the line
        -- for every subsequent handler, so rewrites compose in priority
        -- order instead of last-writer-wins on the original text.
        local line = select(1, ...)

        for _, entry in ipairs(handlers) do
            if registry:active(entry) then
                local result = run_handler(entry, line)
                if result == false then
                    return "", false  -- gagged
                elseif type(result) == "string" then
                    line = rune.line.new(result)
                end
                -- nil = pass through unchanged
            end
        end

        return line:raw(), true

    elseif event == "echo" then
        -- Echo receives the typed text as a plain string. Rewrites
        -- chain like output/prompt; false hides the echo entirely.
        local text = select(1, ...)
        for _, entry in ipairs(handlers) do
            if registry:active(entry) then
                local result = run_handler(entry, text)
                if result == false then
                    return "", false
                elseif type(result) == "string" then
                    text = result
                end
            end
        end
        return text, true

    elseif event == "input" then
        -- Any handler returning false stops processing
        local text = select(1, ...)
        local context = select(2, ...)
        local mode = context and context.mode or "command"
        for _, entry in ipairs(handlers) do
            if registry:active(entry) then
                -- Existing one-argument handlers remain valid in Lua; they
                -- simply ignore the submission context.
                local result = run_handler(entry, text, input_context(mode))
                if result == false then
                    return false  -- consumed/stopped
                end
            end
        end
        return true

    else
        -- System events (notifications) - all handlers run
        local args = {...}
        for _, entry in ipairs(handlers) do
            if registry:active(entry) then
                run_handler(entry, unpack(args))
            end
        end
    end
end

-- List all registered handlers
function rune.hooks.list()
    local result = {}
    for _, entry in ipairs(registry:items()) do
        table.insert(result, {
            event = entry.event,
            name = entry.name,
            group = entry.group,
            priority = entry.priority,
            enabled = entry.enabled,
            source = entry.source,
        })
    end
    return result
end

-- Clear all handlers for an event (or all if no event specified)
function rune.hooks.clear(event)
    if event then
        local handlers = by_event[event]
        if not handlers then
            return
        end
        local handles = {}
        for _, entry in ipairs(handlers) do
            handles[#handles + 1] = entry._handle
        end
        for _, handle in ipairs(handles) do
            handle:remove()
        end
    else
        registry:clear()
    end
end

-- Check if any handlers are registered for an event
function rune.hooks.has(event)
    return by_event[event] ~= nil and #by_event[event] > 0
end

-- Count handlers
function rune.hooks.count(event)
    if event then
        return by_event[event] and #by_event[event] or 0
    end
    return registry:count()
end

-- Group operations
function rune.hooks.remove_group(group_name)
    return registry:remove_group(group_name)
end
