-- Command sequences, single-command repeats, and alias expansion.

local MAX_RECURSION_DEPTH = 100

local function send_impl(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
        return false
    end

    local separator = rune.config.get("command_separator")
    -- Reject the removed repeat-block syntax before sending any of this input.
    -- Otherwise saved scripts could execute fragments of their former bodies.
    for source in rune.input._commands(input, separator) do
        if source:match("^%s*#%d+%s*{") then
            rune.echo(rune.style.red("[Error]") ..
                " Command blocks are not supported; repeat an alias with #N name instead")
            return false
        end
    end

    for source, command in rune.input._commands(input, separator) do
        command = command:match("^%s*(.-)%s*$")
        -- Recognize the prefix before decoding can introduce a literal '#'.
        local count = source:match("^%s*#(%d+)%s+%S")
        local line = count and command:match("^#%d+%s+(.-)%s*$") or command

        for _ = 1, tonumber(count) or 1 do
            if line == "" then
                if not rune.send_raw(line) then return false end
            else
                local processed, result, failed = rune.alias.process(line)
                if failed then return false end
                if not processed then
                    if not rune.send_raw(line) then return false end
                elseif result then
                    if not send_impl(result, depth + 1) then return false end
                end
            end
        end
    end
    return true
end

-- PUBLIC: Send commands to the MUD
function rune.send(input)
    send_impl(input, 0)
end

-- INTERNAL: Route one submission after input hooks and history commit.
-- Programmatic rune.send deliberately enters below this boundary.
function rune.input._dispatch(input, mode)
    if mode == "verbatim" then
        return rune.send_raw(input) ~= nil
    end

    -- Check for slash command first. Dispatch runs the handler under
    -- its own quarantine, so a broken command is disabled individually
    -- instead of breaking the terminal dispatcher.
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        local found, ok = rune.command.dispatch(cmd, args)
        if not found then
            rune.echo(rune.style.red("[Error]") .. " Unknown command: /" .. cmd)
            return false
        end
        return ok
    end

    return send_impl(input, 0)
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
