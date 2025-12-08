-- Hook Registry System
-- Allows multiple handlers per event with priority ordering.
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
-- Events:
--   "input"       -- User input (return false to consume)
--   "output"      -- Server output (return false to gag, string to modify)
--   "prompt"      -- Server prompt (return false to gag, string to modify)
--   "connected"   -- After connection established
--   "disconnected"-- After disconnection
--   "reloading"   -- Before script reload
--   "reloaded"    -- After script reload
--   "error"       -- On system error

-- Handle metatable
local Handle = {}
Handle.__index = Handle

function Handle:remove()
    local data = self._data
    local event_handlers = registry[data.event]
    if event_handlers then
        for i, entry in ipairs(event_handlers) do
            if entry.id == data.id then
                table.remove(event_handlers, i)
                break
            end
        end
    end

    -- Remove from name lookup
    if data.name and by_name[data.name] == self then
        by_name[data.name] = nil
    end

    -- Remove from group
    if data.group and by_group[data.group] then
        by_group[data.group][self] = nil
    end

    return self
end

function Handle:disable()
    self._data.enabled = false
    return self
end

function Handle:enable()
    self._data.enabled = true
    return self
end

function Handle:name()
    return self._data.name
end

function Handle:group()
    return self._data.group
end

-- Internal registries
local registry = {}   -- { event = { {id, handler, priority, enabled, ...}, ... } }
local by_name = {}    -- name -> handle
local by_group = {}   -- group -> {handle -> true, ...}
local next_id = 1

-- Sort handlers by priority (lower first), then by insertion order
local function sort_handlers(handlers)
    table.sort(handlers, function(a, b)
        if a.priority == b.priority then
            return a.id < b.id
        end
        return a.priority < b.priority
    end)
end

-- Public API
rune.hooks = {}

-- Attach a handler to an event
-- Returns: Handle with :remove(), :enable(), :disable(), :name(), :group()
function rune.hooks.on(event, handler, options)
    options = options or {}

    if not registry[event] then
        registry[event] = {}
    end

    local id = next_id
    next_id = next_id + 1

    local entry = {
        id = id,
        event = event,
        handler = handler,
        priority = options.priority or 50,
        enabled = true,
        name = options.name,
        group = options.group,
    }

    local handle = setmetatable({
        _data = entry,
    }, Handle)

    -- Upsert: remove existing with same name
    if entry.name and by_name[entry.name] then
        by_name[entry.name]:remove()
    end

    table.insert(registry[event], entry)
    sort_handlers(registry[event])

    -- Track by name
    if entry.name then
        by_name[entry.name] = handle
    end

    -- Track by group
    if entry.group then
        if not by_group[entry.group] then
            by_group[entry.group] = {}
        end
        by_group[entry.group][handle] = true
    end

    -- Store handle reference in entry
    entry._handle = handle

    return handle
end

-- Management by name
function rune.hooks.disable(name)
    local handle = by_name[name]
    if handle then
        handle:disable()
        return true
    end
    return false
end

function rune.hooks.enable(name)
    local handle = by_name[name]
    if handle then
        handle:enable()
        return true
    end
    return false
end

function rune.hooks.remove(name)
    local handle = by_name[name]
    if handle then
        handle:remove()
        return true
    end
    return false
end

-- Call all handlers for an event
-- For output/prompt: chains modifications, false gags
-- For input: false stops processing
-- For sys events: all handlers run (notifications)
--
-- Return semantics for handlers:
--   return false    -> Stop/Gag
--   return string   -> Modify, pass to next
--   return nil      -> Pass through unmodified
function rune.hooks.call(event, ...)
    local handlers = registry[event]
    if not handlers or #handlers == 0 then
        -- No handlers registered
        if event == "output" then
            -- Line object - return raw text
            local line = select(1, ...)
            return line:raw(), true
        elseif event == "prompt" then
            local line = select(1, ...)
            return line:raw(), true
        elseif event == "input" then
            return true
        end
        return
    end

    -- Determine event type for return value handling
    if event == "output" then
        -- Output receives a Line object (userdata with :raw() and :clean() methods)
        -- Chain handlers, passing the Line object to each
        local line = select(1, ...)
        local modified_text = nil  -- Track if any handler modified the output

        for _, entry in ipairs(handlers) do
            if entry.enabled then
                local result = entry.handler(line)
                if result == false then
                    return "", false  -- gagged
                elseif type(result) == "string" then
                    modified_text = result  -- handler returned modified text
                end
                -- nil = pass through unchanged
            end
        end

        -- Return modified text if any handler changed it, otherwise raw line
        if modified_text then
            return modified_text, true
        end
        return line:raw(), true

    elseif event == "prompt" then
        -- Prompt receives a Line object (like output)
        local line = select(1, ...)
        local modified_text = nil

        for _, entry in ipairs(handlers) do
            if entry.enabled then
                local result = entry.handler(line)
                if result == false then
                    return "", false  -- gagged
                elseif type(result) == "string" then
                    modified_text = result  -- modified
                end
                -- nil = pass through unchanged
            end
        end

        if modified_text then
            return modified_text, true
        end
        return line:raw(), true

    elseif event == "input" then
        -- Any handler returning false stops processing
        local text = select(1, ...)
        for _, entry in ipairs(handlers) do
            if entry.enabled then
                local result = entry.handler(text)
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
            if entry.enabled then
                entry.handler(unpack(args))
            end
        end
    end
end

-- List all registered handlers
function rune.hooks.list()
    local result = {}

    for event, handlers in pairs(registry) do
        for _, entry in ipairs(handlers) do
            table.insert(result, {
                event = event,
                name = entry.name,
                group = entry.group,
                priority = entry.priority,
                enabled = entry.enabled,
            })
        end
    end

    return result
end

-- Clear all handlers for an event (or all if no event specified)
function rune.hooks.clear(event)
    if event then
        -- Remove from by_name and by_group first
        local handlers = registry[event]
        if handlers then
            for _, entry in ipairs(handlers) do
                if entry.name then
                    by_name[entry.name] = nil
                end
                if entry.group and by_group[entry.group] then
                    by_group[entry.group][entry._handle] = nil
                end
            end
        end
        registry[event] = {}
    else
        registry = {}
        by_name = {}
        by_group = {}
    end
end

-- Check if any handlers are registered for an event
function rune.hooks.has(event)
    return registry[event] and #registry[event] > 0
end

-- Count handlers
function rune.hooks.count(event)
    if event then
        return registry[event] and #registry[event] or 0
    end
    local total = 0
    for _, handlers in pairs(registry) do
        total = total + #handlers
    end
    return total
end

-- Group operations
function rune.hooks.remove_group(group_name)
    if not group_name or not by_group[group_name] then
        return 0
    end
    local count = 0
    -- Copy to avoid modifying while iterating
    local items = {}
    for handle in pairs(by_group[group_name]) do
        items[#items + 1] = handle
    end
    for _, handle in ipairs(items) do
        handle:remove()
        count = count + 1
    end
    return count
end
