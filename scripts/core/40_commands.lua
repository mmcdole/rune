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
        rune.print("\027[31m[Error] Alias loop detected (depth limit exceeded)\027[0m")
        return
    end

    local commands = expand_input(input)

    for _, line in ipairs(commands) do
        if line ~= "" then
            local cmd, args = line:match("^(%S+)%s*(.*)")
            cmd = cmd or line
            args = args or ""

            local alias = rune.alias.get(cmd)

            if alias then
                if type(alias) == "function" then
                    -- Function aliases are a new execution context
                    local ok, err = pcall(alias, args)
                    if not ok then
                        rune.print("\027[31m[Error] Alias '" .. cmd .. "' failed: " .. tostring(err) .. "\027[0m")
                    end
                else
                    -- String alias: recursively expand
                    local expansion = alias
                    if args ~= "" then
                        expansion = expansion .. " " .. args
                    end
                    send_impl(expansion, depth + 1)
                end
            else
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
rune.hooks.register("input", function(input)
    -- Check for slash command first
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        local handler = rune.command.get(cmd)
        if handler then
            handler(args)
        else
            rune.print("[Error] Unknown command: /" .. cmd)
        end
        return false
    end

    -- Process as normal command
    rune.send(input)
    return false
end, { priority = 100 })

-- Register output handler
rune.hooks.register("output", function(line)
    local modified, show = rune.trigger.process(line)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })

-- Register prompt handler
rune.hooks.register("prompt", function(line)
    local modified, show = rune.trigger.process(line)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })
