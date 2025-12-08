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
