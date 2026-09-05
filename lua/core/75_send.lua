-- Command Processing System
-- Simple recursion-based design: no queues, no global state.

local MAX_RECURSION_DEPTH = 100

-- Expand repeats into commands, never back into text to split a second time.
local function expand_input(input, separator)
    local commands = {}
    for source, command in rune.input._commands(input, separator) do
        local count, body = source:match("^%s*#(%d+)%s*{([^}]+)}%s*$")
        if count then
            local repeated = expand_input(body, separator)
            for _ = 1, tonumber(count) do
                for _, line in ipairs(repeated) do
                    commands[#commands + 1] = line
                end
            end
        else
            command = command:match("^%s*(.-)%s*$")
            local times = source:match("^%s*#(%d+)%s+[^{}]+%s*$")
            local line = times and command:match("^#%d+%s+(.-)%s*$") or command
            for _ = 1, tonumber(times) or 1 do
                commands[#commands + 1] = line
            end
        end
    end
    return commands
end

-- INTERNAL: Recursive send implementation
local function send_impl(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
        return
    end

    local commands = expand_input(input, rune.config.get("command_separator"))

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
