-- Trigger System
-- Triggers match server output and execute actions.
--
-- API (literal matching):
--   rune.trigger.exact(line, action, opts?)      -- Exact line match (literal)
--   rune.trigger.starts(prefix, action, opts?)   -- Prefix match (literal)
--   rune.trigger.contains(substr, action, opts?) -- Substring match (literal)
--
-- API (regex matching):
--   rune.trigger.regex(pattern, action, opts?)   -- Go regexp match
--
-- Returns a handle with :disable(), :enable(), :remove(), :name(), :group()
--
-- Options:
--   name     = "string"   -- Unique ID for upsert/management
--   group    = "string"   -- Group membership for bulk operations
--   once     = true       -- Auto-remove after first match
--   priority = 50         -- Execution order (lower = first)
--   gag      = true       -- Hide matching line
--   raw      = true       -- Match against raw line (with ANSI codes)
--
-- Action can be:
--   - String: sent as command, %1 %2 etc substituted from captures (regex only)
--   - Function: function(matches, ctx)
--       matches = array of captures (empty for literal modes, populated for regex)
--       ctx = {line, name, group, type, matches}

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

function Handle:name()
    return self._data.name
end

function Handle:group()
    return self._data.group
end

-- Internal registry
local registry = {
    list = {},        -- All triggers, ordered by priority then insertion
    by_name = {},     -- name -> handle
    by_group = {},    -- group -> {handle -> true, ...}
}

-- Sort triggers by priority (lower first), then by insertion order
local function sort_triggers()
    table.sort(registry.list, function(a, b)
        if a.priority ~= b.priority then
            return a.priority < b.priority
        end
        return a.id < b.id
    end)
end

-- ID counter for insertion order
local next_id = 1

-- Match modes
local MODE_EXACT = "exact"
local MODE_STARTS = "starts"
local MODE_CONTAINS = "contains"
local MODE_REGEX = "regex"

-- Create a trigger (internal)
local function create_trigger(pattern, action, opts, mode)
    opts = opts or {}

    local data = {
        id = next_id,
        pattern = pattern,
        action = action,
        mode = mode,
        enabled = true,
        priority = opts.priority or 50,
        name = opts.name,
        group = opts.group,
        once = opts.once or false,
        gag = opts.gag or false,
        raw = opts.raw or false,
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

    -- Add to main list
    table.insert(registry.list, data)
    sort_triggers()

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
rune.trigger = {}

-- Exact line match (literal, whole line)
function rune.trigger.exact(line, action, opts)
    return create_trigger(line, action, opts, MODE_EXACT)
end

-- Prefix match (literal, line starts with)
function rune.trigger.starts(prefix, action, opts)
    return create_trigger(prefix, action, opts, MODE_STARTS)
end

-- Substring match (literal, line contains)
function rune.trigger.contains(substring, action, opts)
    return create_trigger(substring, action, opts, MODE_CONTAINS)
end

-- Go regexp match
function rune.trigger.regex(pattern, action, opts)
    return create_trigger(pattern, action, opts, MODE_REGEX)
end

-- Management by name
function rune.trigger.disable(name)
    local handle = registry.by_name[name]
    if handle then
        handle:disable()
        return true
    end
    return false
end

function rune.trigger.enable(name)
    local handle = registry.by_name[name]
    if handle then
        handle:enable()
        return true
    end
    return false
end

function rune.trigger.remove(name)
    local handle = registry.by_name[name]
    if handle then
        handle:remove()
        return true
    end
    return false
end

-- List all triggers - returns array of {match, value, mode, name, enabled, ...}
function rune.trigger.list()
    local result = {}

    for _, data in ipairs(registry.list) do
        table.insert(result, {
            match = data.pattern,
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            mode = data.mode,
            name = data.name,
            enabled = data.enabled,
            group = data.group,
            gag = data.gag,
            once = data.once,
            raw = data.raw,
        })
    end

    return result
end

-- Clear all triggers
function rune.trigger.clear()
    registry.list = {}
    registry.by_name = {}
    registry.by_group = {}
end

-- Count triggers
function rune.trigger.count()
    return #registry.list
end

-- Process triggers against a line (called by hooks)
-- Returns: modified_text (string), show (bool)
function rune.trigger.process(line)
    local gagged = false
    local modified_text = nil

    local raw_line = line:raw()
    local clean_line = line:clean()

    -- Collect triggers to remove after processing (for once)
    local to_remove = {}

    for _, data in ipairs(registry.list) do
        -- Check individual state AND group master switch
        if data.enabled and rune.group.is_enabled(data.group) then
            local matches = nil
            local match_line = data.raw and raw_line or clean_line

            if data.mode == MODE_EXACT then
                -- Exact line match (literal)
                if match_line == data.pattern then
                    matches = {}
                end
            elseif data.mode == MODE_STARTS then
                -- Prefix match (literal)
                if match_line:sub(1, #data.pattern) == data.pattern then
                    matches = {}
                end
            elseif data.mode == MODE_CONTAINS then
                -- Substring match (literal)
                if match_line:find(data.pattern, 1, true) then
                    matches = {}
                end
            elseif data.mode == MODE_REGEX then
                -- Go regexp match
                matches = rune.regex.match(data.pattern, match_line)
            end

            if matches then
                -- Handle gag option
                if data.gag then
                    gagged = true
                end

                -- Build context
                local ctx = {
                    line = line,  -- Line object with :raw() and :clean()
                    name = data.name,
                    group = data.group,
                    type = "trigger",
                    matches = matches,
                }

                -- Execute action
                if data.action then
                    if type(data.action) == "function" then
                        local ok, result = pcall(data.action, matches, ctx)
                        if not ok then
                            rune.echo("[Trigger Error] " .. tostring(result))
                        else
                            -- Handle return values
                            if result == false then
                                gagged = true
                            elseif type(result) == "string" then
                                modified_text = result
                            end
                        end
                    elseif type(data.action) == "string" and data.action ~= "" then
                        -- String action with capture substitution
                        local cmd = data.action
                        for i, m in ipairs(matches) do
                            cmd = cmd:gsub("%%" .. i, m)
                        end
                        rune.send(cmd)
                    end
                end

                -- Handle once
                if data.once then
                    to_remove[#to_remove + 1] = data._handle
                end
            end
        end
    end

    -- Remove once triggers
    for _, handle in ipairs(to_remove) do
        handle:remove()
    end

    if gagged then
        return "", false
    end
    return modified_text or raw_line, true
end

-- Group operations
function rune.trigger.remove_group(group_name)
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

