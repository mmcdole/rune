-- TinTin++ Compatibility Layer
-- Provides support for TinTin++ syntax patterns

-- Expand #N {commands} syntax into repeated commands
-- e.g., "#6 north" becomes "north;north;north;north;north;north"
-- e.g., "#3 {kill rat;loot}" becomes "kill rat;loot;kill rat;loot;kill rat;loot"
local function expandRepeats(input)
    local result = input

    -- Handle #N {braced content}
    result = result:gsub("#(%d+)%s*{([^}]+)}", function(count, content)
        local n = tonumber(count)
        local expanded = {}
        for i = 1, n do
            table.insert(expanded, content)
        end
        return table.concat(expanded, ";")
    end)

    -- Handle #N single_command (word until ; or end)
    result = result:gsub("#(%d+)%s+([^;{]+)", function(count, content)
        local n = tonumber(count)
        local cmd = content:match("^%s*(.-)%s*$") -- trim
        local expanded = {}
        for i = 1, n do
            table.insert(expanded, cmd)
        end
        return table.concat(expanded, ";")
    end)

    return result
end

-- Register the preprocessor
rune.tintin = {
    expandRepeats = expandRepeats
}
