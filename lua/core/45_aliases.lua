-- Alias System
-- Aliases match user input and transform/expand it.
-- Built on rune.registry (20_registry.lua).
--
-- API (literal matching):
--   rune.alias.exact(phrase, action, opts?)   -- Match a literal command phrase
--
-- API (regex matching):
--   rune.alias.regex(pattern, action, opts?)  -- Go regexp on full input line
--
-- Returns a handle with :disable(), :enable(), :remove(), :name(), :group()
--
-- Options:
--   name     = "string"   -- Management name; an exact alias is also
--                            addressed by its phrase (see 20_registry.lua)
--   group    = "string"   -- Group membership for bulk operations
--   once     = true       -- Auto-remove after first match
--   priority = 50         -- Execution order for regex aliases (lower = first)
--
-- Action can be:
--   - String: expansion text, %1 %2 etc substituted from captures (regex only)
--   - Function (exact):  function(args, ctx)  -- args = text after matched phrase
--   - Function (regex):  function(matches, ctx) -- matches = array of captures
--
-- Context object:
--   ctx.line  = full input line
--   ctx.name  = alias name (the phrase for an unnamed exact alias)
--   ctx.group = alias group (if set)
--   ctx.type  = "alias"
--   ctx.args  = text after matched phrase (exact only)
--   ctx.matches = captures array (regex only)

-- Exact-command index: a word trie lets dispatch follow only registered
-- prefixes and stop at the first impossible continuation.
local exact_root = { children = {} }

local function index_exact(data)
    local node = exact_root
    local words = {}
    for word in data.pattern:gmatch("%S+") do
        words[#words + 1] = word
        local child = node.children[word]
        if not child then
            child = { children = {} }
            node.children[word] = child
        end
        node = child
    end
    node.alias = data
    data._exact_words = words
end

local function unindex_exact(data)
    local words = data._exact_words
    if not words then
        return
    end

    local node = exact_root
    local path = { node }
    for _, word in ipairs(words) do
        node = node.children[word]
        if not node then
            data._exact_words = nil
            return
        end
        path[#path + 1] = node
    end

    if node.alias == data then
        node.alias = nil
    end
    for i = #words, 1, -1 do
        local child = path[i + 1]
        if child.alias == nil and next(child.children) == nil then
            path[i].children[words[i]] = nil
        else
            break
        end
    end
    data._exact_words = nil
end

local registry = rune.registry.new{
    kind = "alias",
    action_field = "action",
    on_add = function(data)
        if data.is_exact then
            -- The phrase is the alias's key, so any previous alias on
            -- it is already gone by now.
            index_exact(data)
        end
    end,
    on_remove = function(data)
        if data.is_exact then
            unindex_exact(data)
        end
    end,
}

-- Create an alias (internal). Raises in the caller's caller on a
-- refused registration.
local function create_alias(label, pattern, action, opts, is_exact)
    if opts ~= nil and type(opts) ~= "table" then
        error(label .. ": opts must be a table", 3)
    end
    local handle, err = registry:add({
        key = is_exact and pattern or nil,
        pattern = pattern,
        action = action,
        is_exact = is_exact,
        source = rune.caller_source(2),
    }, opts)
    if not handle then
        error(label .. ": " .. err, 3)
    end
    return handle
end

-- Public API
rune.alias = {}

-- Match a literal command phrase. Whitespace separates words rather
-- than being part of the phrase, matching the input parser's behavior.
-- The normalized phrase is the alias's key, so rune.alias.get("gc")
-- and rune.alias.disable("gc") address it by what you typed to make it.
function rune.alias.exact(phrase, action, opts)
    if type(phrase) ~= "string" then
        error("rune.alias.exact: phrase must be a string", 2)
    end
    local normalized = phrase:gsub("%s+", " ")
    normalized = normalized:gsub("^ ", ""):gsub(" $", "")
    if normalized == "" then
        error("rune.alias.exact: phrase must contain at least one word", 2)
    end
    return create_alias("rune.alias.exact", normalized, action, opts, true)
end

-- Go regexp match on full input line
-- Raises on an invalid pattern so typos fail loudly at registration,
-- with the caller's file:line, instead of never matching.
function rune.alias.regex(pattern, action, opts)
    local ok, err = rune.regex.validate(pattern)
    if not ok then
        error("invalid alias pattern '" .. tostring(pattern) .. "': " .. tostring(err), 2)
    end
    return create_alias("rune.alias.regex", pattern, action, opts, false)
end

-- Management by name or exact phrase
function rune.alias.get(address)
    return registry:get(address)
end

function rune.alias.disable(address)
    return registry:disable(address)
end

function rune.alias.enable(address)
    return registry:enable(address)
end

function rune.alias.remove(address)
    return registry:remove(address)
end

-- List all aliases - returns array of {match, mode, name, value, enabled, ...}
-- Exact aliases first (sorted by phrase), then regex aliases in priority order.
function rune.alias.list()
    local function describe(data)
        return {
            match = data.pattern,
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            mode = data.is_exact and "exact" or "regex",
            name = data._handle:name(),
            enabled = data.enabled,
            group = data.group,
            once = data.once,
            source = data.source,
        }
    end

    local exacts, regexes = {}, {}
    for _, data in ipairs(registry:items()) do
        local bucket = data.is_exact and exacts or regexes
        bucket[#bucket + 1] = describe(data)
    end
    table.sort(exacts, function(a, b) return a.match < b.match end)
    for _, item in ipairs(regexes) do
        exacts[#exacts + 1] = item
    end
    return exacts
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
                        name = data._handle:name(),
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

    -- Then walk the exact-alias trie. Retaining the last active candidate
    -- makes the most specific (longest) phrase win.
    local winner, winner_end = nil, nil
    local node = exact_root
    local token_start, token_end = input:find("%S+")
    if token_start == 1 then
        while token_start do
            local token = input:sub(token_start, token_end)
            node = node.children[token]
            if not node then
                break
            end
            local candidate = node.alias
            if candidate and registry:active(candidate) then
                winner = candidate
                winner_end = token_end
            end
            token_start, token_end = input:find("%S+", token_end + 1)
        end
    end
    if winner then
        local args_start = input:find("%S", winner_end + 1)
        local args = args_start and input:sub(args_start) or ""
        local result = nil

        if type(winner.action) == "function" then
            local ctx = {
                line = input,
                name = winner._handle:name(),
                group = winner.group,
                type = "alias",
                args = args,
            }
            result = run_action(winner, args, ctx)
        elseif type(winner.action) == "string" then
            if args and args ~= "" then
                result = winner.action .. " " .. args
            else
                result = winner.action
            end
        end

        if winner.once then
            winner._handle:remove()
        end

        return true, result
    end

    return false, nil
end

-- Group operations
function rune.alias.remove_group(group_name)
    return registry:remove_group(group_name)
end
