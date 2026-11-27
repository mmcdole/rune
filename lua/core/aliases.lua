-- Alias System
-- Aliases match user input and transform/expand it.
-- Built on rune.registry (registry.lua).
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
--   name     = "string"   -- Unique ID for upsert/management
--   group    = "string"   -- Group membership for bulk operations
--   once     = true       -- Auto-remove after first match
--   priority = 50         -- Execution order for regex aliases (lower = first)
--
-- Exact aliases are tried first; regex aliases only when none matches.
--
-- Action can be:
--   - String: expansion text, %1 %2 etc substituted from captures (regex only)
--   - Function (exact):  function(args, ctx)  -- args = text after matched phrase
--   - Function (regex):  function(matches, ctx) -- matches = array of captures
--
-- Context object:
--   ctx.line  = full input line
--   ctx.name  = alias name (if set)
--   ctx.group = alias group (if set)
--   ctx.type  = "alias"
--   ctx.args  = text after matched phrase (exact only)
--   ctx.matches = captures array (regex only)

-- Exact-command index: a word trie, so dispatch follows only registered
-- prefixes and stops at the first impossible continuation. (An exact alias
-- is named for its phrase, so the registry's name index is the phrase map.)
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
    on_add = function(data)
        if data.is_exact then
            index_exact(data)
        end
    end,
    on_remove = function(data)
        if data.is_exact then
            unindex_exact(data)
        end
    end,
}

-- Create an alias (internal)
local function create_alias(pattern, action, opts, is_exact)
    local api = is_exact and "rune.alias.exact" or "rune.alias.regex"
    if type(action) ~= "function" and type(action) ~= "string" then
        rune._raise(api .. ": action must be a function or string")
    end
    return registry:add({
        pattern = pattern,
        action = action,
        is_exact = is_exact,
        source = rune.caller_source(2),
    }, opts, api)
end

-- Public API
rune.alias = registry:expose({})

-- Match a literal command phrase. Whitespace separates words rather
-- than being part of the phrase, matching the input parser's behavior.
-- The normalized phrase is the alias's name, so rune.alias.get("gc")
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
    return create_alias(normalized, action,
        rune.registry.keyed_opts(normalized, opts, "rune.alias.exact"), true)
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

-- List all aliases - returns array of {match, mode, name, value, enabled, ...}
-- Exact aliases first (sorted by key), then regex aliases in priority order.
function rune.alias.list()
    local rows = registry:list(function(row, data)
        row.match = data.pattern
        row.value = type(data.action) == "function" and "(function)" or tostring(data.action)
        row.mode = data.is_exact and "exact" or "regex"
        row.once = data.once
    end)
    local exact, regex = {}, {}
    for _, row in ipairs(rows) do
        local bucket = row.mode == "exact" and exact or regex
        bucket[#bucket + 1] = row
    end
    table.sort(exact, function(a, b) return a.match < b.match end)
    for _, row in ipairs(regex) do
        exact[#exact + 1] = row
    end
    return exact
end

-- Run an alias action protected, with quarantine: an action failing
-- repeatedly is disabled like any hook/trigger/timer action.
-- Returns the action's result, or nil on failure.
local function run_action(data, arg, ctx)
    local ok, result = registry:call(data, arg, ctx)
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
    -- First walk the exact-alias trie: a named phrase beats a pattern that
    -- happens to match it. Retaining the last active candidate makes the
    -- most specific (longest) phrase win.
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
                name = winner.name,
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

    -- Then try regex aliases (priority order) - uses Go regexp
    for _, data in ipairs(registry:snapshot()) do
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

    return false, nil
end
