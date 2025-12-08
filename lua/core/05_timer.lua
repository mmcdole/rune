-- Timer System
-- Timers execute actions after a delay or repeatedly at intervals.
--
-- API:
--   rune.timer.after(seconds, action, opts?)  -- One-shot timer
--   rune.timer.every(seconds, action, opts?)  -- Repeating timer
--
-- every() schedules the next firing seconds after the previous firing completes.
--
-- Returns a handle with :disable(), :enable(), :cancel(), :name(), :group()
--
-- Options:
--   name  = "string"  -- Unique ID for upsert/management
--   group = "string"  -- Group membership for bulk operations
--
-- Action can be:
--   - String: sent as command
--   - Function: function(ctx)
--       ctx = {name, group, type, cancel()}

-- Handle metatable
local Handle = {}
Handle.__index = Handle

function Handle:disable()
    self._data.enabled = false
    return self
end

function Handle:enable()
    self._data.enabled = true
    return self
end

function Handle:cancel()
    local data = self._data
    local registry = self._registry

    -- Cancel the underlying timer
    if data.id then
        rune._timer.cancel(data.id)
    end

    -- Remove from main list
    for i, item in ipairs(registry.list) do
        if item == data then
            table.remove(registry.list, i)
            break
        end
    end

    -- Remove from name lookup
    if data.name and registry.by_name[data.name] == self then
        registry.by_name[data.name] = nil
    end

    -- Remove from group
    if data.group and registry.by_group[data.group] then
        registry.by_group[data.group][self] = nil
    end

    return self
end

-- Alias for consistency with aliases/triggers
Handle.remove = Handle.cancel

function Handle:name()
    return self._data.name
end

function Handle:group()
    return self._data.group
end

-- Internal registry
local registry = {
    list = {},        -- All timers
    by_name = {},     -- name -> handle
    by_group = {},    -- group -> {handle -> true, ...}
}

-- Create a timer (internal)
local function create_timer(seconds, action, opts, repeating)
    opts = opts or {}

    local data = {
        id = nil,  -- Set after scheduling
        seconds = seconds,
        action = action,
        repeating = repeating,
        enabled = true,
        name = opts.name,
        group = opts.group,
    }

    local handle = setmetatable({
        _data = data,
        _registry = registry,
    }, Handle)

    -- If named, remove existing with same name (upsert)
    if data.name and registry.by_name[data.name] then
        registry.by_name[data.name]:cancel()
    end

    -- Build the callback wrapper
    local function callback()
        -- Check individual state
        if not data.enabled then
            return
        end
        -- Check group master switch
        if not rune.group.is_enabled(data.group) then
            return
        end

        -- Build context
        local ctx = {
            name = data.name,
            group = data.group,
            type = "timer",
        }
        function ctx:cancel()
            handle:cancel()
        end

        -- Execute action
        if type(data.action) == "function" then
            local ok, err = pcall(data.action, ctx)
            if not ok then
                rune.echo("[Timer Error] " .. tostring(err))
            end
        elseif type(data.action) == "string" and data.action ~= "" then
            rune.send(data.action)
        end

        -- Auto-remove one-shot timers after firing
        if not data.repeating then
            handle:cancel()
        end
    end

    -- Schedule the timer
    if repeating then
        data.id = rune._timer.every(seconds, callback)
    else
        data.id = rune._timer.after(seconds, callback)
    end

    -- Add to main list
    table.insert(registry.list, data)

    -- Add to name lookup
    if data.name then
        registry.by_name[data.name] = handle
    end

    -- Add to group
    if data.group then
        if not registry.by_group[data.group] then
            registry.by_group[data.group] = {}
        end
        registry.by_group[data.group][handle] = true
    end

    -- Store handle reference in data
    data._handle = handle

    return handle
end

-- Public API
rune.timer = {}

-- One-shot timer (fires once after delay)
function rune.timer.after(seconds, action, opts)
    return create_timer(seconds, action, opts, false)
end

-- Repeating timer (fires every interval)
function rune.timer.every(seconds, action, opts)
    return create_timer(seconds, action, opts, true)
end

-- Management by name
function rune.timer.disable(name)
    local handle = registry.by_name[name]
    if handle then
        handle:disable()
        return true
    end
    return false
end

function rune.timer.enable(name)
    local handle = registry.by_name[name]
    if handle then
        handle:enable()
        return true
    end
    return false
end

function rune.timer.cancel(name)
    local handle = registry.by_name[name]
    if handle then
        handle:cancel()
        return true
    end
    return false
end

-- Alias for consistency with aliases/triggers
rune.timer.remove = rune.timer.cancel

-- List all timers - returns array of {seconds, mode, value, name, enabled, group}
function rune.timer.list()
    local result = {}

    for _, data in ipairs(registry.list) do
        table.insert(result, {
            seconds = data.seconds,
            mode = data.repeating and "every" or "after",
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            name = data.name,
            enabled = data.enabled,
            group = data.group,
        })
    end

    return result
end

-- Clear all timers
function rune.timer.clear()
    -- Copy to avoid modifying while iterating
    local items = {}
    for _, data in ipairs(registry.list) do
        items[#items + 1] = data._handle
    end
    for _, handle in ipairs(items) do
        handle:cancel()
    end
end

-- Count timers
function rune.timer.count()
    return #registry.list
end

-- Group operations
function rune.timer.remove_group(group_name)
    if not group_name or not registry.by_group[group_name] then
        return 0
    end
    local count = 0
    -- Copy to avoid modifying while iterating
    local items = {}
    for handle in pairs(registry.by_group[group_name]) do
        items[#items + 1] = handle
    end
    for _, handle in ipairs(items) do
        handle:cancel()
        count = count + 1
    end
    return count
end

-- Regex API

rune.regex = {}

-- Pattern cache for compiled regexes
local regex_cache = {}

-- Returns Regex userdata with :match(text) method, or nil + error
function rune.regex.compile(pattern)
    return rune._regex.compile(pattern)
end

-- Match pattern against text, return captures array or nil
-- Caches compiled patterns for performance
function rune.regex.match(pattern, text)
    -- Get or compile regex
    local re = regex_cache[pattern]
    if not re then
        local err
        re, err = rune._regex.compile(pattern)
        if not re then
            return nil
        end
        regex_cache[pattern] = re
    end

    -- Match and extract captures (skip index 1 which is full match)
    local matches = re:match(text)
    if not matches then
        return nil
    end

    -- Return captures only (index 2+), or empty table if no captures
    local captures = {}
    for i = 2, #matches do
        captures[i - 1] = matches[i]
    end
    return captures
end

-- Pane API

rune.pane = {}

function rune.pane.create(name)
    rune._pane.create(name)
end

function rune.pane.write(name, text)
    rune._pane.write(name, text)
end

function rune.pane.toggle(name)
    rune._pane.toggle(name)
end

function rune.pane.clear(name)
    rune._pane.clear(name)
end
