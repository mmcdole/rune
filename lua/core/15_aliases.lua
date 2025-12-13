-- Alias System
-- Aliases match user input and transform/expand it.
--
-- API (literal matching):
--   rune.alias.exact(key, action, opts?)      -- Match command word exactly (literal)
--
-- API (regex matching):
--   rune.alias.regex(pattern, action, opts?)  -- Go regexp on full input line
--
-- Returns a handle with :disable(), :enable(), :remove(), :name(), :group()
--
-- Options:
--   name     = "string"   -- Unique ID for upsert/management
--   group    = "string"   -- Group membership for bulk operations
--   once     = true       -- Auto-remove after first match
--   priority = 50         -- Execution order for regex aliases (lower = first)
--
-- Action can be:
--   - String: expansion text, %1 %2 etc substituted from captures (regex only)
--   - Function (exact):  function(args, ctx)  -- args = string after command word
--   - Function (regex):  function(matches, ctx) -- matches = array of captures
--
-- Context object:
--   ctx.line  = full input line
--   ctx.name  = alias name (if set)
--   ctx.group = alias group (if set)
--   ctx.type  = "alias"
--   ctx.args  = args string (exact only)
--   ctx.matches = captures array (regex only)

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

function Handle:remove()
    local data = self._data
    local registry = self._registry

    if data.is_exact then
        -- Remove from exact lookup
        if registry.exact[data.pattern] == data then
            registry.exact[data.pattern] = nil
        end
    else
        -- Remove from regex list
        for i, item in ipairs(registry.regex) do
            if item == data then
                table.remove(registry.regex, i)
                break
            end
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

function Handle:name()
    return self._data.name
end

function Handle:group()
    return self._data.group
end

-- Internal registry
local registry = {
    exact = {},       -- key -> data (for fast command lookup)
    regex = {},       -- Regex aliases, ordered by priority
    by_name = {},     -- name -> handle
    by_group = {},    -- group -> {handle -> true, ...}
}

-- Sort regex aliases by priority (lower first), then by insertion order
local function sort_regex()
    table.sort(registry.regex, function(a, b)
        if a.priority ~= b.priority then
            return a.priority < b.priority
        end
        return a.id < b.id
    end)
end

-- ID counter for insertion order
local next_id = 1

-- Create an alias (internal)
local function create_alias(pattern, action, opts, is_exact)
    opts = opts or {}

    local data = {
        id = next_id,
        pattern = pattern,
        action = action,
        is_exact = is_exact,
        enabled = true,
        priority = opts.priority or 50,
        name = opts.name,
        group = opts.group,
        once = opts.once or false,
    }
    next_id = next_id + 1

    local handle = setmetatable({
        _data = data,
        _registry = registry,
    }, Handle)

    -- If named, remove existing with same name (upsert)
    if data.name and registry.by_name[data.name] then
        registry.by_name[data.name]:remove()
    end

    -- Add to appropriate storage
    if is_exact then
        -- For exact aliases, also remove existing with same key (upsert by key)
        if registry.exact[data.pattern] then
            local old_data = registry.exact[data.pattern]
            if old_data._handle then
                -- Clean up old handle's name/group refs
                if old_data.name and registry.by_name[old_data.name] == old_data._handle then
                    registry.by_name[old_data.name] = nil
                end
                if old_data.group and registry.by_group[old_data.group] then
                    registry.by_group[old_data.group][old_data._handle] = nil
                end
            end
        end
        registry.exact[data.pattern] = data
    else
        table.insert(registry.regex, data)
        sort_regex()
    end

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

    -- Store handle reference in data for removal
    data._handle = handle

    return handle
end

-- Public API
rune.alias = {}

-- Match command word exactly (first word of input, literal)
function rune.alias.exact(command, action, opts)
    return create_alias(command, action, opts, true)
end

-- Go regexp match on full input line
function rune.alias.regex(pattern, action, opts)
    return create_alias(pattern, action, opts, false)
end

-- Management by name
function rune.alias.disable(name)
    local handle = registry.by_name[name]
    if handle then
        handle:disable()
        return true
    end
    return false
end

function rune.alias.enable(name)
    local handle = registry.by_name[name]
    if handle then
        handle:enable()
        return true
    end
    return false
end

function rune.alias.remove(name)
    local handle = registry.by_name[name]
    if handle then
        handle:remove()
        return true
    end
    return false
end

-- List all aliases - returns array of {match, mode, name, value, enabled, ...}
function rune.alias.list()
    local result = {}

    -- Exact aliases
    local exact_keys = {}
    for k in pairs(registry.exact) do
        exact_keys[#exact_keys + 1] = k
    end
    table.sort(exact_keys)

    for _, key in ipairs(exact_keys) do
        local data = registry.exact[key]
        table.insert(result, {
            match = key,
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            mode = "exact",
            name = data.name,
            enabled = data.enabled,
            group = data.group,
            once = data.once,
        })
    end

    -- Regex aliases
    for _, data in ipairs(registry.regex) do
        table.insert(result, {
            match = data.pattern,
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            mode = "regex",
            name = data.name,
            enabled = data.enabled,
            group = data.group,
            once = data.once,
        })
    end

    return result
end

-- Clear all aliases
function rune.alias.clear()
    registry.exact = {}
    registry.regex = {}
    registry.by_name = {}
    registry.by_group = {}
end

-- Count aliases
function rune.alias.count()
    local count = 0
    for _ in pairs(registry.exact) do
        count = count + 1
    end
    return count + #registry.regex
end

-- Process input through aliases
-- Returns: processed (bool), result (string or nil)
-- If processed is true and result is nil, input was consumed by function alias
-- If processed is true and result is string, use result as new input
-- If processed is false, no alias matched
function rune.alias.process(input)
    -- First try regex aliases (priority order) - uses Go regexp
    for _, data in ipairs(registry.regex) do
        -- Check individual state AND group master switch
        if data.enabled and rune.group.is_enabled(data.group) then
            local matches = rune.regex.match(data.pattern, input)
            if matches then
                local result = nil

                if type(data.action) == "function" then
                    local ctx = {
                        line = input,
                        name = data.name,
                        group = data.group,
                        type = "alias",
                        matches = matches,
                    }
                    local ok, ret = pcall(data.action, matches, ctx)
                    if not ok then
                        rune.echo("[Alias Error] " .. tostring(ret))
                    else
                        result = ret
                    end
                elseif type(data.action) == "string" then
                    result = data.action
                    for i, m in ipairs(matches) do
                        result = result:gsub("%%" .. i, m)
                    end
                end

                -- Handle once
                if data.once and data._handle then
                    data._handle:remove()
                end

                return true, result
            end
        end
    end

    -- Then try exact aliases (command word match)
    local cmd, args = input:match("^(%S+)%s*(.*)")
    if cmd then
        local data = registry.exact[cmd]
        -- Check individual state AND group master switch
        if data and data.enabled and rune.group.is_enabled(data.group) then
            local result = nil

            if type(data.action) == "function" then
                -- For exact aliases, pass args string (not matches array)
                local ctx = {
                    line = input,
                    name = data.name,
                    group = data.group,
                    type = "alias",
                    args = args,
                }
                local ok, ret = pcall(data.action, args, ctx)
                if not ok then
                    rune.echo("[Alias Error] " .. tostring(ret))
                else
                    result = ret
                end
            elseif type(data.action) == "string" then
                -- Exact alias expansion: append args
                if args and args ~= "" then
                    result = data.action .. " " .. args
                else
                    result = data.action
                end
            end

            -- Handle once
            if data.once and data._handle then
                data._handle:remove()
            end

            return true, result
        end
    end

    return false, nil
end

-- Group operations
function rune.alias.remove_group(group_name)
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
        handle:remove()
        count = count + 1
    end
    return count
end

