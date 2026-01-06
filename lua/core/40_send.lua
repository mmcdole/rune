-- Command Processing System
-- Simple recursion-based design: no queues, no global state.

local MAX_RECURSION_DEPTH = 100

-- INTERNAL: Expand #N repeat syntax
-- e.g., "#6 north" becomes "north;north;north;north;north;north"
-- e.g., "#3 {kill rat;loot}" becomes "kill rat;loot;kill rat;loot;kill rat;loot"
local function expand_repeats(input)
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

-- INTERNAL: Expand repeats and split by delimiter
local function expand_input(input)
    -- 1. Handle #N repeat syntax
    input = expand_repeats(input)

    -- 2. Split by delimiter
    local commands = {}
    if input == "" then return {""} end

    local delimiter = rune.config.delimiter
    local start = 1
    while true do
        local pos = input:find(delimiter, start, true)
        if not pos then
            table.insert(commands, input:sub(start):match("^%s*(.-)%s*$"))
            break
        end
        table.insert(commands, input:sub(start, pos - 1):match("^%s*(.-)%s*$"))
        start = pos + #delimiter
    end
    return commands
end

-- INTERNAL: Recursive send implementation
local function send_impl(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo("\027[31m[Error] Alias loop detected (depth limit exceeded)\027[0m")
        return
    end

    local commands = expand_input(input)

    for _, line in ipairs(commands) do
        if line == "" then
            -- Empty command - send it directly
            rune.send_raw(line)
        else
            -- Try alias expansion (pattern aliases first, then exact aliases)
            local processed, result = rune.alias.process(line)

            if processed then
                if result then
                    -- Alias returned a string - recursively expand
                    send_impl(result, depth + 1)
                end
                -- If result is nil, alias was a function that handled everything
            else
                -- No alias matched - send directly
                rune.send_raw(line)
            end
        end
    end
end

-- PUBLIC: Send commands to the MUD
function rune.send(input)
    send_impl(input, 0)
end

-- Register input handler
rune.hooks.on("input", function(input)
    -- Check for slash command first
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        local handler = rune.command.get(cmd)
        if handler then
            handler(args)
        else
            rune.echo("[Error] Unknown command: /" .. cmd)
        end
        return false
    end

    -- Process as normal command
    rune.send(input)
    return false
end, { priority = 100 })

-- Register output handler
rune.hooks.on("output", function(line)
    local modified, show = rune.trigger.process(line)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })

-- Register prompt handler
rune.hooks.on("prompt", function(line)
    local modified, show = rune.trigger.process(line)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })
