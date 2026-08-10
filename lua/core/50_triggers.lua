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
--   once     = true       -- Auto-remove after first match (spans: first fire)
--   priority = 50         -- Execution order (lower = first)
--   gag      = true       -- Hide matching line (spans: every collected line)
--   raw      = true       -- Match against raw line (with ANSI codes)
--   on       = "output"   -- "output" (default) or live "prompt" updates
--   confirmed_only = true -- Prompt triggers: require Telnet GA/EOR
--   span     = {          -- Collect a multi-line message; action fires once
--     to  = "regex",      --   line that ends the span, inclusive (optional)
--     raw = true,         --   match `to` against the raw line
--     max = 8,            --   flush after N lines, header included
--   }
--
-- Action can be:
--   - String: sent as command, %1 %2 etc substituted from captures (regex only)
--   - Function: function(matches, ctx)
--       matches = array of captures (empty for literal modes, populated for regex)
--       ctx = {line, name, group, type, matches, prompt_confirmed?}
--       Span actions also get ctx.text (joined message) and ctx.lines
--       (collected line objects); their return values are ignored, since
--       the collected lines have already been displayed.

local registry = rune.registry.new{ kind = "trigger" }

-- Match modes
local MODE_EXACT = "exact"
local MODE_STARTS = "starts"
local MODE_CONTAINS = "contains"
local MODE_REGEX = "regex"

local CHANNEL_OUTPUT = "output"
local CHANNEL_PROMPT = "prompt"

-- Create a trigger (internal)
local function create_trigger(pattern, action, opts, mode)
    opts = opts or {}

    local channel = opts.on
    if channel == nil then
        channel = CHANNEL_OUTPUT
    end
    if type(channel) ~= "string" or
            (channel ~= CHANNEL_OUTPUT and channel ~= CHANNEL_PROMPT) then
        error('trigger on must be "output" or "prompt"', 3)
    end

    local confirmed_only = opts.confirmed_only
    if confirmed_only == nil then
        confirmed_only = false
    elseif type(confirmed_only) ~= "boolean" then
        error("trigger confirmed_only must be a boolean", 3)
    elseif confirmed_only and channel ~= CHANNEL_PROMPT then
        error('trigger confirmed_only requires on = "prompt"', 3)
    end

    if opts.span ~= nil and channel ~= CHANNEL_OUTPUT then
        error('trigger span requires on = "output"', 3)
    end

    local span = nil
    if opts.span ~= nil then
        if type(opts.span) ~= "table" then
            error("trigger span must be a table", 3)
        end
        local to = opts.span.to
        if to ~= nil then
            if type(to) ~= "string" then
                error("trigger span.to must be a string pattern", 3)
            end
            local ok, err = rune.regex.validate(to)
            if not ok then
                error("invalid span.to pattern '" .. tostring(to) .. "': " .. tostring(err), 3)
            end
        end
        local max = opts.span.max
        if max == nil then
            max = 8
        elseif type(max) ~= "number" or max < 1 or max % 1 ~= 0 then
            error("trigger span.max must be an integer >= 1", 3)
        end
        -- Copy, so callers mutating their opts table later can't skew
        -- an already-registered trigger.
        span = { to = to, raw = not not opts.span.raw, max = max }
    end

    return registry:add({
        pattern = pattern,
        action = action,
        mode = mode,
        gag = opts.gag or false,
        raw = opts.raw or false,
        channel = channel,
        confirmed_only = confirmed_only,
        span = span,
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
            on = data.channel,
            confirmed_only = data.confirmed_only,
            span = data.span and { to = data.span.to, raw = data.span.raw, max = data.span.max } or nil,
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

-- Match one trigger against a line; returns the captures array on a
-- match (empty for literal modes), or nil.
local function match_header(data, match_line)
    if data.mode == MODE_EXACT then
        if match_line == data.pattern then
            return {}
        end
    elseif data.mode == MODE_STARTS then
        if match_line:sub(1, #data.pattern) == data.pattern then
            return {}
        end
    elseif data.mode == MODE_CONTAINS then
        if match_line:find(data.pattern, 1, true) then
            return {}
        end
    elseif data.mode == MODE_REGEX then
        return rune.regex.match(data.pattern, match_line)
    end
    return nil
end

local function trim(s)
    return (s:gsub("%s+$", ""))
end

-- Open spans: data table ref -> {lines, pieces, matches}. Module
-- state, so /reload drops any in-progress span with the VM.
local open = {}

local function trigger_label(data)
    return (data.name and ('Trigger "' .. data.name .. '"') or "Trigger") ..
        (data.source and (" @" .. data.source) or "")
end

local function clean_open_spans()
    for data in pairs(open) do
        if not registry:active(data) then
            open[data] = nil
        end
    end
end

-- Fire a completed (or explicitly flushed) span. Return values are ignored:
-- every collected line has already passed through the display pipeline.
local function fire_span(data, st, to_remove)
    local ctx = {
        line = st.lines[1],
        lines = st.lines,
        text = table.concat(st.pieces, " "),
        name = data.name,
        group = data.group,
        type = "trigger",
        matches = st.matches,
    }
    if type(data.action) == "function" then
        rune.guarded_call(trigger_label(data), data, data.action, st.matches, ctx)
    elseif type(data.action) == "string" and data.action ~= "" then
        rune.send(rune.substitute_captures(data.action, st.matches))
    end
    if data.once then
        to_remove[#to_remove + 1] = data._handle
    end
end

-- A confirmed Telnet prompt, user submission, or outbound command is an
-- explicit boundary for incomplete multiline records. Partial snapshots never
-- call this function.
function rune.trigger._flush_spans()
    clean_open_spans()
    local to_remove = {}
    for _, data in ipairs(registry:snapshot()) do
        local st = open[data]
        if st then
            open[data] = nil
            -- An earlier span action may have removed, disabled, or muted a
            -- later trigger from this snapshot. Honor that mutation just as
            -- normal trigger dispatch does.
            if registry:active(data) then
                fire_span(data, st, to_remove)
            end
        end
    end
    for _, handle in ipairs(to_remove) do
        handle:remove()
    end
end

-- Drop incomplete multiline records without running their actions or
-- removing once-triggers. Connection lifecycle changes use this when the
-- collected lines no longer belong to the active server stream.
function rune.trigger._discard_spans()
    open = {}
end

local function new_span(data, matches, line, clean_line)
    local piece = (data.mode == MODE_REGEX and #matches > 0)
        and matches[#matches] or clean_line
    local st = { lines = { line }, pieces = {}, matches = matches }
    piece = trim(piece)
    if piece ~= "" then
        st.pieces[1] = piece
    end
    return st
end

local function span_ends(data, st, raw_line, clean_line)
    local sp = data.span
    if sp.to and rune.regex.match(sp.to, sp.raw and raw_line or clean_line) then
        return true
    end
    return #st.lines >= sp.max
end

-- Process one trigger channel. Output is the safe default and contains only
-- delimiter-terminated lines. Prompt is opt-in and receives both unfinished
-- snapshots and GA/EOR-confirmed prompts; prompt actions may therefore run
-- more than once as the same visible tail grows.
function rune.trigger.process(line, channel, prompt_confirmed)
    channel = channel or CHANNEL_OUTPUT
    if channel ~= CHANNEL_OUTPUT and channel ~= CHANNEL_PROMPT then
        error('trigger process channel must be "output" or "prompt"', 2)
    end
    if channel == CHANNEL_PROMPT and type(prompt_confirmed) ~= "boolean" then
        error("prompt trigger processing requires prompt_confirmed", 2)
    end

    clean_open_spans()

    local gagged = false
    local modified_text = nil
    local raw_line = line:raw()
    local clean_line = line:clean()
    local to_remove = {}

    -- A declaratively gagged output match must not flash speculatively in the
    -- prompt overlay. This is a read-only projection: no output trigger action
    -- runs, and no span opens, grows, closes, or fires from unfinished text.
    if channel == CHANNEL_PROMPT and not prompt_confirmed then
        for _, data in ipairs(registry:snapshot()) do
            if registry:active(data) and data.channel == CHANNEL_OUTPUT and
                    data.gag then
                local match_line = data.raw and raw_line or clean_line
                if (data.span and open[data]) or match_header(data, match_line) then
                    gagged = true
                end
            end
        end
    end

    -- Snapshot: a trigger action that adds/removes triggers must not
    -- perturb this dispatch pass (removals still honored via active()).
    for _, data in ipairs(registry:snapshot()) do
        if registry:active(data) and data.channel == channel and
                (not data.confirmed_only or prompt_confirmed) then
            local match_line = data.raw and raw_line or clean_line

            if open[data] then
                -- This line belongs to the open span; it cannot also
                -- header-match the same trigger in this pass.
                if data.gag then
                    gagged = true
                end
                local matches = match_header(data, match_line)
                local st
                if matches then
                    -- New header mid-span: flush the previous message
                    -- and start over.
                    fire_span(data, open[data], to_remove)
                    open[data] = nil
                    st = new_span(data, matches, line, clean_line)
                else
                    st = open[data]
                    st.lines[#st.lines + 1] = line
                    local piece = trim(clean_line)
                    if piece ~= "" then
                        st.pieces[#st.pieces + 1] = piece
                    end
                end
                if span_ends(data, st, raw_line, clean_line) then
                    fire_span(data, st, to_remove)
                    open[data] = nil
                else
                    open[data] = st
                end
            else
                local matches = match_header(data, match_line)
                if matches then
                    if data.span then
                        if data.gag then
                            gagged = true
                        end
                        local st = new_span(data, matches, line, clean_line)
                        if span_ends(data, st, raw_line, clean_line) then
                            fire_span(data, st, to_remove) -- complete on one line
                        else
                            open[data] = st
                        end
                    else
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
                        if channel == CHANNEL_PROMPT then
                            ctx.prompt_confirmed = prompt_confirmed
                        end

                        if data.action then
                            if type(data.action) == "function" then
                                local ok, result = rune.guarded_call(trigger_label(data), data, data.action, matches, ctx)
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
