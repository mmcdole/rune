-- Hook Registry System
-- Allows multiple handlers per event with priority ordering.
-- Built on rune.registry (20_registry.lua) for handles, names,
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
--   "input"        -- User input: (text, context); strings rewrite, false consumes
--   "output"       -- Server output line object (false gags, string rewrites)
--   "prompt"       -- Prompt observation: (line, confirmed); confirmed is
--                     false for a partial line, true for a GA/EOR-confirmed
--                     prompt (false gags, string rewrites)
--   "echo"         -- Local echo of final input after rewrites, plain string (false hides,
--                     string rewrites; core handler adds the "> " styling)
-- Events (notifications):
--   "ready"        -- Core and user scripts loaded; staged settings not yet applied
--   "connecting"   -- Dial started
--   "connected"    -- After connection established
--   "disconnecting"-- Disconnect requested
--   "disconnected" -- After disconnection
--   "reloading"    -- Before script reload
--   "reloaded"     -- After script reload
--   "loaded"       -- After a script file loads
--   "error"        -- On system error
--   "input_changed"-- Input buffer content changed
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
    action_field = "handler",
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
function rune.hooks.get(name)
    return registry:get(name)
end

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

-- Give each input handler its own read-only context.
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

-- Output and prompt chain Line objects; strings replace the object seen by
-- later handlers, and false gags it. Only prompt receives a second argument.
local function line_chain(event, handlers, line, confirmed)
    if event == "prompt" and #handlers > 0 and type(confirmed) ~= "boolean" then
        error("prompt requires an explicit confirmed boolean", 2)
    end
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            local result
            if event == "prompt" then
                result = run_handler(entry, line, confirmed)
            else
                result = run_handler(entry, line)
            end
            if result == false then
                return "", false
            elseif type(result) == "string" then
                line = rune.line.new(result)
            end
        end
    end
    return line:raw(), true
end

-- Echo chains display text; false hides the local echo.
local function text_chain(_, handlers, text)
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
end

-- Input chains physical lines. Reject newlines before subsequent handlers
-- can act on them, and isolate each handler's view of the canonical mode.
local function run_input_hooks(_, handlers, text, context)
    local mode = context and context.mode or "command"
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            local result = run_handler(entry, text, input_context(mode))
            if result == false then
                return false
            elseif type(result) == "string" then
                if result:find("[\r\n]") then
                    error("input rewrite must stay on one line", 0)
                end
                text = result
            end
        end
    end
    return text
end

-- Notifications preserve the full argument list, including embedded/trailing
-- nil values. Handler return values do not affect subsequent notifications.
local function notify(_, handlers, ...)
    local nargs = select("#", ...)
    local args = {...}
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            run_handler(entry, unpack(args, 1, nargs))
        end
    end
end

local chains = {
    output = line_chain,
    prompt = line_chain,
    echo = text_chain,
    input = run_input_hooks,
}

-- Snapshot membership before calling user code. Additions wait for the next
-- dispatch; removals are honored by each chain's registry:active check.
function rune.hooks.call(event, ...)
    local handlers = {}
    for i, entry in ipairs(by_event[event] or {}) do
        handlers[i] = entry
    end
    return (chains[event] or notify)(event, handlers, ...)
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
