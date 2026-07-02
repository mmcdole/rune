-- Alias System
-- Aliases match user input and transform/expand it.
-- Built on rune.registry (06_registry.lua).
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

-- Exact-command index: command word -> data, kept in sync with the
-- registry so exact lookup stays O(1).
local exact = {}

local registry = rune.registry.new{
    kind = "alias",
    on_add = function(data)
        if data.is_exact then
            -- Upsert by command word: replace any previous exact alias
            local old = exact[data.pattern]
            if old and old ~= data then
                old._handle:remove()
            end
            exact[data.pattern] = data
        end
    end,
    on_remove = function(data)
        if data.is_exact and exact[data.pattern] == data then
            exact[data.pattern] = nil
        end
    end,
}

-- Create an alias (internal)
local function create_alias(pattern, action, opts, is_exact)
    return registry:add({
        pattern = pattern,
        action = action,
        is_exact = is_exact,
        source = rune.caller_source(2),
    }, opts)
end

-- Public API
rune.alias = {}

-- Match command word exactly (first word of input, literal)
function rune.alias.exact(command, action, opts)
    return create_alias(command, action, opts, true)
end

-- Go regexp match on full input line
-- Raises on an invalid pattern so typos fail loudly at registration,
-- with the caller's file:line, instead of never matching.
function rune.alias.regex(pattern, action, opts)
    local ok, err = rune.regex.validate(pattern)
    if not ok then
        error("invalid alias pattern '" .. tostring(pattern) .. "': " .. tostring(err), 2)
    end
    return create_alias(pattern, action, opts, false)
end

-- Management by name
function rune.alias.disable(name)
    return registry:disable(name)
end

function rune.alias.enable(name)
    return registry:enable(name)
end

function rune.alias.remove(name)
    return registry:remove(name)
end

-- List all aliases - returns array of {match, mode, name, value, enabled, ...}
-- Exact aliases first (sorted by key), then regex aliases in priority order.
function rune.alias.list()
    local function describe(data)
        return {
            match = data.pattern,
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            mode = data.is_exact and "exact" or "regex",
            name = data.name,
            enabled = data.enabled,
            group = data.group,
            once = data.once,
            source = data.source,
        }
    end

    local result = {}

    local exact_keys = {}
    for k in pairs(exact) do
        exact_keys[#exact_keys + 1] = k
    end
    table.sort(exact_keys)
    for _, key in ipairs(exact_keys) do
        table.insert(result, describe(exact[key]))
    end

    for _, data in ipairs(registry:items()) do
        if not data.is_exact then
            table.insert(result, describe(data))
        end
    end

    return result
end

-- Clear all aliases
function rune.alias.clear()
    registry:clear()
end

-- Count aliases
function rune.alias.count()
    return registry:count()
end

-- Run an alias action protected, with quarantine: an action failing
-- repeatedly is disabled like any hook/trigger/timer action.
-- Returns the action's result, or nil on failure.
local function run_action(data, arg, ctx)
    local label = 'Alias "' .. tostring(data.name or data.pattern) .. '"' ..
        (data.source and (" @" .. data.source) or "")
    local ok, result = rune.guarded_call(label, data, data.action, arg, ctx)
    if ok then
        return result
    end
    return nil
end

-- Process input through aliases
-- Returns: processed (bool), result (string or nil)
-- If processed is true and result is nil, input was consumed by function alias
-- If processed is true and result is string, use result as new input
-- If processed is false, no alias matched
function rune.alias.process(input)
    -- First try regex aliases (priority order) - uses Go regexp
    for _, data in ipairs(registry:items()) do
        if not data.is_exact and registry:active(data) then
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
                    result = run_action(data, matches, ctx)
                elseif type(data.action) == "string" then
                    result = rune.substitute_captures(data.action, matches)
                end

                if data.once then
                    data._handle:remove()
                end

                return true, result
            end
        end
    end

    -- Then try exact aliases (command word match)
    local cmd, args = input:match("^(%S+)%s*(.*)")
    if cmd then
        local data = exact[cmd]
        if data and registry:active(data) then
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
                result = run_action(data, args, ctx)
            elseif type(data.action) == "string" then
                -- Exact alias expansion: append args
                if args and args ~= "" then
                    result = data.action .. " " .. args
                else
                    result = data.action
                end
            end

            if data.once then
                data._handle:remove()
            end

            return true, result
        end
    end

    return false, nil
end

-- Group operations
function rune.alias.remove_group(group_name)
    return registry:remove_group(group_name)
end
