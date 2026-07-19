-- Command Processing System
-- Simple recursion-based design: no queues, no global state.

local MAX_RECURSION_DEPTH = 100

-- INTERNAL: Expand #N repeat syntax
-- e.g., "#6 north" becomes "north;north;north;north;north;north"
-- e.g., "#3 {kill rat;loot}" becomes "kill rat;loot;kill rat;loot;kill rat;loot"
--
-- Repeats are anchored at command position (start of input, or right
-- after a delimiter): "#3 north" is a repeat, but "say #3 cheers" is
-- chat text and passes through untouched. The temporary leading ";"
-- lets one pattern cover both anchor cases.
local function expand_repeats(input)
    local result = ";" .. input

    -- Handle #N {braced content}
    result = result:gsub(";%s*#(%d+)%s*{([^}]+)}", function(count, content)
        local n = tonumber(count)
        local expanded = {}
        for i = 1, n do
            table.insert(expanded, content)
        end
        return ";" .. table.concat(expanded, ";")
    end)

    -- Handle #N single_command (text until ; or end)
    result = result:gsub(";%s*#(%d+)%s+([^;{]+)", function(count, content)
        local n = tonumber(count)
        local cmd = content:match("^%s*(.-)%s*$") -- trim
        local expanded = {}
        for i = 1, n do
            table.insert(expanded, cmd)
        end
        return ";" .. table.concat(expanded, ";")
    end)

    return result:sub(2)
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
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
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

-- Send an entire submission without interpreting any of it as a Rune command.
-- Only LF separates outbound lines; whitespace, CR bytes, delimiters, repeats,
-- slash commands, and empty lines remain data. This is private core policy,
-- closed over by the input hook rather than exposed on a Go-owned rune._ table.
local function send_verbatim(input)
    local start = 1
    while true do
        local pos = input:find("\n", start, true)
        if not pos then
            rune.send_raw(input:sub(start))
            return
        end
        rune.send_raw(input:sub(start, pos - 1))
        start = pos + 1
    end
end

-- PUBLIC: Send commands to the MUD
function rune.send(input)
    send_impl(input, 0)
end

-- Register input handler
rune.hooks.on("input", function(input, context)
    -- Verbatim is a submission policy, not a separate lifecycle: earlier
    -- input hooks still observe it and may consume it, but none of Rune's
    -- command syntax is applied once it reaches this core handler.
    if context.mode == "verbatim" then
        send_verbatim(input)
        return false
    end

    -- Check for slash command first. Dispatch runs the handler under
    -- its own quarantine, so a broken command is disabled individually
    -- instead of its failures accruing against this core hook.
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        if not rune.command.dispatch(cmd, args) then
            rune.echo(rune.style.red("[Error]") .. " Unknown command: /" .. cmd)
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

-- Register prompt handler. is_prompt = true: a prompt is never part
-- of a multi-line span, so any open span flushes first.
rune.hooks.on("prompt", function(line)
    local modified, show = rune.trigger.process(line, true)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })
