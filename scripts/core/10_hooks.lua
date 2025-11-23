-- Hook Registry System
-- Allows multiple handlers per event with priority ordering

rune.hooks = {}
local registry = {}  -- { event = { {id, handler, priority}, ... } }
local next_id = 1

-- Register a handler for an event
-- Returns: handler ID for later removal
-- Options: { priority = number } (lower = earlier, default 100)
function rune.hooks.register(event, handler, options)
    options = options or {}
    local priority = options.priority or 100

    if not registry[event] then
        registry[event] = {}
    end

    local id = next_id
    next_id = next_id + 1

    table.insert(registry[event], {
        id = id,
        handler = handler,
        priority = priority,
    })

    -- Sort by priority (stable sort preserves insertion order for equal priorities)
    table.sort(registry[event], function(a, b)
        if a.priority == b.priority then
            return a.id < b.id
        end
        return a.priority < b.priority
    end)

    return id
end

-- Remove a handler by ID
function rune.hooks.remove(id)
    for event, handlers in pairs(registry) do
        for i, entry in ipairs(handlers) do
            if entry.id == id then
                table.remove(handlers, i)
                return true
            end
        end
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
        if event == "output" or event == "prompt" then
            return select(1, ...), true
        elseif event == "input" then
            return true
        end
        return
    end

    -- Determine event type for return value handling
    if event == "output" or event == "prompt" then
        -- Chain modifications
        local current = select(1, ...)
        for _, entry in ipairs(handlers) do
            local result = entry.handler(current)
            if result == false then
                return "", false  -- gagged
            elseif type(result) == "string" then
                current = result  -- modified
            end
            -- nil = pass through unchanged
        end
        return current, true

    elseif event == "input" then
        -- Any handler returning false stops processing
        local text = select(1, ...)
        for _, entry in ipairs(handlers) do
            local result = entry.handler(text)
            if result == false then
                return false  -- consumed/stopped
            end
        end
        return true

    else
        -- System events (notifications) - all handlers run
        local args = {...}
        for _, entry in ipairs(handlers) do
            entry.handler(table.unpack(args))
        end
    end
end

-- List all registered handlers (for debugging)
function rune.hooks.list()
    rune.print("[Hooks]")
    local count = 0
    for event, handlers in pairs(registry) do
        for _, entry in ipairs(handlers) do
            rune.print("  " .. event .. " #" .. entry.id .. " (priority " .. entry.priority .. ")")
            count = count + 1
        end
    end
    if count == 0 then
        rune.print("  (none)")
    end
end

-- Clear all handlers for an event (or all if no event specified)
function rune.hooks.clear(event)
    if event then
        registry[event] = {}
    else
        registry = {}
    end
end

-- Check if any handlers are registered for an event
function rune.hooks.has(event)
    return registry[event] and #registry[event] > 0
end
