-- Group System (Control Only)
-- Manages the master enable/disable state for groups.
-- Item deletion is handled by each module (alias, trigger, timer, hooks).
--
-- Two-level enable/disable:
--   - Group level: master switch (rune.group.disable/enable)
--   - Item level: individual state (handle:disable/enable)
--
-- An item fires only if BOTH are enabled.
-- Group disable doesn't mutate individual states - they're preserved for re-enable.

rune.group = {}

-- Master switch state: group_name -> bool (nil = enabled)
rune.group._states = {}

-- Check if a group is enabled (used by alias/trigger/timer modules)
function rune.group.is_enabled(group_name)
    if not group_name then return true end
    if rune.group._states[group_name] == false then
        return false
    end
    return true
end

-- Disable a group (master switch off)
function rune.group.disable(group_name)
    if not group_name then return end
    rune.group._states[group_name] = false
end

-- Enable a group (master switch on)
function rune.group.enable(group_name)
    if not group_name then return end
    rune.group._states[group_name] = true
end

-- List all known groups (aggregated from all modules)
-- Returns array of {name, enabled}
function rune.group.list()
    local seen = {}
    local result = {}

    -- Collect groups from aliases
    if rune.alias and rune.alias.list then
        for _, item in ipairs(rune.alias.list()) do
            if item.group and not seen[item.group] then
                seen[item.group] = true
            end
        end
    end

    -- Collect groups from triggers
    if rune.trigger and rune.trigger.list then
        for _, item in ipairs(rune.trigger.list()) do
            if item.group and not seen[item.group] then
                seen[item.group] = true
            end
        end
    end

    -- Collect groups from timers
    if rune.timer and rune.timer.list then
        for _, item in ipairs(rune.timer.list()) do
            if item.group and not seen[item.group] then
                seen[item.group] = true
            end
        end
    end

    -- Collect groups from hooks
    if rune.hooks and rune.hooks.list then
        for _, item in ipairs(rune.hooks.list()) do
            if item.group and not seen[item.group] then
                seen[item.group] = true
            end
        end
    end

    -- Also include any explicitly disabled groups (even if empty)
    for group_name, _ in pairs(rune.group._states) do
        if not seen[group_name] then
            seen[group_name] = true
        end
    end

    -- Build result array
    for group_name, _ in pairs(seen) do
        table.insert(result, {
            name = group_name,
            enabled = rune.group.is_enabled(group_name),
        })
    end

    -- Sort by name
    table.sort(result, function(a, b) return a.name < b.name end)
    return result
end
