-- Trigger System

rune.trigger = {}
local storage = {}   -- { id = { pattern, action, enabled, gag, is_regex } }
local order = {}     -- List for ordered execution
local next_id = 1    -- Auto-incrementing ID

-- Add a trigger
-- Returns: trigger ID for later removal
-- action can be a string (command to send) or a function (callback)
-- options: { gag = bool, enabled = bool, regex = bool }
function rune.trigger.add(pattern, action, options)
    options = options or {}

    local id = next_id
    next_id = next_id + 1

    storage[id] = {
        pattern = pattern,
        action = action,
        enabled = options.enabled ~= false,  -- default true
        gag = options.gag or false,
        is_regex = options.regex or false,   -- Use Go regex instead of Lua patterns
    }
    table.insert(order, id)
    return id
end

-- Remove a trigger by ID or pattern
function rune.trigger.remove(id_or_pattern)
    local id = id_or_pattern

    -- If string, find by pattern
    if type(id_or_pattern) == "string" then
        for tid, trig in pairs(storage) do
            if trig.pattern == id_or_pattern then
                id = tid
                break
            end
        end
    end

    if storage[id] then
        local pattern = storage[id].pattern
        storage[id] = nil
        -- Remove from order list
        for i, oid in ipairs(order) do
            if oid == id then
                table.remove(order, i)
                break
            end
        end
        rune.print("[Trigger] Removed #" .. id .. ": " .. pattern)
    else
        rune.print("[Trigger] Not found: " .. tostring(id_or_pattern))
    end
end

-- Clear all triggers
function rune.trigger.clear()
    storage = {}
    order = {}
    next_id = 1
end

-- Count triggers
function rune.trigger.count()
    return #order
end

-- Enable/disable a trigger by ID or pattern
function rune.trigger.enable(id_or_pattern, enabled)
    local id = id_or_pattern

    -- If string, find by pattern
    if type(id_or_pattern) == "string" then
        for tid, trig in pairs(storage) do
            if trig.pattern == id_or_pattern then
                id = tid
                break
            end
        end
    end

    if storage[id] then
        storage[id].enabled = enabled
        local status = enabled and "enabled" or "disabled"
        rune.print("[Trigger] #" .. id .. " " .. status)
    else
        rune.print("[Trigger] Not found: " .. tostring(id_or_pattern))
    end
end

-- List all triggers
function rune.trigger.list()
    rune.print("[Triggers]")
    local count = 0
    for _, id in ipairs(order) do
        local trig = storage[id]
        if trig then
            local status = trig.enabled and "on" or "off"
            local gag = trig.gag and " [gag]" or ""
            local action_desc
            if type(trig.action) == "string" then
                action_desc = trig.action
            else
                action_desc = "(function)"
            end
            rune.print("  [" .. status .. "] #" .. id .. ": /" .. trig.pattern .. "/ -> " .. action_desc .. gag)
            count = count + 1
        end
    end
    if count == 0 then
        rune.print("  (none)")
    end
end

-- Process triggers against a line
-- Returns: modified_line, should_display
function rune.trigger.process(line)
    local gagged = false  -- track if line should be gagged

    -- Iterate using the ORDER list
    for _, id in ipairs(order) do
        local trig = storage[id]
        if trig and trig.enabled then
            local matches = nil

            -- Support both Lua patterns and Go Regex
            if trig.is_regex then
                -- rune.regex.match returns a table of captures {full, group1, group2...}
                matches = rune.regex.match(trig.pattern, line)
            else
                -- Standard Lua match
                local m = {line:match(trig.pattern)}
                if #m > 0 or line:match(trig.pattern) then
                    matches = m
                end
            end

            if matches then
                -- Trigger matched
                if trig.gag then
                    gagged = true
                end

                -- Execute action
                if type(trig.action) == "function" then
                    -- Call function with captures (pass matches table)
                    local result = trig.action(line, matches)
                    -- Function can return:
                    --   false = gag the line
                    --   string = modified line
                    --   nil/true/anything else = keep line as-is
                    if result == false then
                        gagged = true
                    elseif type(result) == "string" then
                        line = result
                    end
                elseif type(trig.action) == "string" then
                    -- Send command (expand with captures using %1, %2, etc.)
                    local cmd = trig.action
                    for i, match in ipairs(matches) do
                        cmd = cmd:gsub("%%" .. i, match)
                    end
                    -- Process the command through rune.send
                    rune.send(cmd)
                end
            end
        end
    end

    if gagged then
        return "", false
    end
    return line, true
end
