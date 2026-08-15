-- Command Processing System
-- Simple recursion-based design: no queues, no global state.

local MAX_RECURSION_DEPTH = 100

-- INTERNAL: Expand braced #N syntax before command splitting. Unbraced
-- repeats expand after splitting so both forms honor an arbitrary configured
-- command separator.
--
-- Repeats are anchored at command position (start of input, or right
-- after a separator): "#3 north" is a repeat, but "say #3 cheers" is
-- chat text and passes through untouched. A temporary leading separator
-- lets one pattern cover both anchor cases for braced groups.
local function expand_braced_repeats(input, separator)
    local escaped_separator = separator:gsub("([^%w])", "%%%1")
    local result = separator .. input

    result = result:gsub(escaped_separator .. "%s*#(%d+)%s*{([^}]+)}", function(count, content)
        local n = tonumber(count)
        local expanded = {}
        for i = 1, n do
            table.insert(expanded, content)
        end
        return separator .. table.concat(expanded, separator)
    end)

    return result:sub(#separator + 1)
end

-- INTERNAL: Expand repeats and split by the command separator.
local function expand_input(input)
    local separator = rune.config.get("command_separator")
    input = expand_braced_repeats(input, separator)

    local commands = {}
    if input == "" then return {""} end

    local start = 1
    while true do
        local pos = input:find(separator, start, true)
        local piece = pos and input:sub(start, pos - 1) or input:sub(start)
        local command = piece:match("^%s*(.-)%s*$")
        local count, repeated = command:match("^#(%d+)%s+([^{}]+)$")
        if count then
            repeated = repeated:match("^%s*(.-)%s*$")
            for _ = 1, tonumber(count) do
                table.insert(commands, repeated)
            end
        else
            table.insert(commands, command)
        end
        if not pos then break end
        start = pos + #separator
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

-- PUBLIC: Send commands to the MUD
function rune.send(input)
    send_impl(input, 0)
end

-- INTERNAL: Route one submission after input hooks and history commit.
-- Programmatic rune.send deliberately enters below this boundary.
function rune.input._dispatch(input, mode)
    if mode == "verbatim" then
        rune.send_raw(input) -- no alias or command interpretation
        return
    end

    -- Check for slash command first. Dispatch runs the handler under
    -- its own quarantine, so a broken command is disabled individually
    -- instead of breaking the terminal dispatcher.
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        if not rune.command.dispatch(cmd, args) then
            rune.echo(rune.style.red("[Error]") .. " Unknown command: /" .. cmd)
        end
        return
    end

    rune.send(input)
end

-- Register output handler
rune.hooks.on("output", function(line)
    local modified, show = rune.trigger._process_output(line)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })

-- Prompt triggers opt into partial lines, which may repeat as they grow.
rune.hooks.on("prompt", function(line, confirmed)
    local modified, show = rune.trigger._process_prompt(line, confirmed)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })
