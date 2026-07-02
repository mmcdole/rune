-- Trigger System
-- Triggers match server output and execute actions.
-- Built on rune.registry (15_registry.lua).
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

local registry = rune.registry.new{ kind = "trigger" }

-- Match modes
local MODE_EXACT = "exact"
local MODE_STARTS = "starts"
local MODE_CONTAINS = "contains"
local MODE_REGEX = "regex"

-- Create a trigger (internal)
local function create_trigger(pattern, action, opts, mode)
    opts = opts or {}
    return registry:add({
        pattern = pattern,
        action = action,
        mode = mode,
        gag = opts.gag or false,
        raw = opts.raw or false,
        source = rune.caller_source(2),
    }, opts)
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
-- Raises on an invalid pattern so typos fail loudly at registration,
-- with the caller's file:line, instead of never firing.
function rune.trigger.regex(pattern, action, opts)
    local ok, err = rune.regex.validate(pattern)
    if not ok then
        error("invalid trigger pattern '" .. tostring(pattern) .. "': " .. tostring(err), 2)
    end
    return create_trigger(pattern, action, opts, MODE_REGEX)
end

-- Management by name
function rune.trigger.disable(name)
    return registry:disable(name)
end

function rune.trigger.enable(name)
    return registry:enable(name)
end

function rune.trigger.remove(name)
    return registry:remove(name)
end

-- List all triggers - returns array of {match, value, mode, name, enabled, ...}
function rune.trigger.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
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
            source = data.source,
        })
    end
    return result
end

-- Clear all triggers
function rune.trigger.clear()
    registry:clear()
end

-- Count triggers
function rune.trigger.count()
    return registry:count()
end

-- Process triggers against a line (called by hooks)
-- Returns: modified_text (string), show (bool)
-- A function action returning a string rewrites the line: later
-- triggers match against (and receive) the rewritten text.
function rune.trigger.process(line)
    local gagged = false
    local modified_text = nil

    local raw_line = line:raw()
    local clean_line = line:clean()

    -- Collect triggers to remove after processing (for once)
    local to_remove = {}

    for _, data in ipairs(registry:items()) do
        if registry:active(data) then
            local matches = nil
            local match_line = data.raw and raw_line or clean_line

            if data.mode == MODE_EXACT then
                if match_line == data.pattern then
                    matches = {}
                end
            elseif data.mode == MODE_STARTS then
                if match_line:sub(1, #data.pattern) == data.pattern then
                    matches = {}
                end
            elseif data.mode == MODE_CONTAINS then
                if match_line:find(data.pattern, 1, true) then
                    matches = {}
                end
            elseif data.mode == MODE_REGEX then
                matches = rune.regex.match(data.pattern, match_line)
            end

            if matches then
                if data.gag then
                    gagged = true
                end

                local ctx = {
                    line = line,  -- Line object with :raw() and :clean()
                    name = data.name,
                    group = data.group,
                    type = "trigger",
                    matches = matches,
                }

                if data.action then
                    if type(data.action) == "function" then
                        local label = (data.name and ('Trigger "' .. data.name .. '"') or "Trigger") ..
                            (data.source and (" @" .. data.source) or "")
                        local ok, result = rune.guarded_call(label, data, data.action, matches, ctx)
                        if ok then
                            if result == false then
                                gagged = true
                            elseif type(result) == "string" then
                                -- Rewrite: later triggers see the new text
                                modified_text = result
                                line = rune.line.new(result)
                                raw_line = line:raw()
                                clean_line = line:clean()
                            end
                        end
                    elseif type(data.action) == "string" and data.action ~= "" then
                        rune.send(rune.substitute_captures(data.action, matches))
                    end
                end

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
    return registry:remove_group(group_name)
end
