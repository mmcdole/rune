-- Command sequences, single-command repeats, and alias expansion.

local MAX_RECURSION_DEPTH = 100

local function send_impl(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
        return
    end

    local separator = rune.config.get("command_separator")
    -- Reject the removed repeat-block syntax before sending any of this input.
    -- Otherwise saved scripts could execute fragments of their former bodies.
    for source in rune.input._commands(input, separator) do
        if source:match("^%s*#%d+%s*{") then
            rune.echo(rune.style.red("[Error]") ..
                " Command blocks are not supported; repeat an alias with #N name instead")
            return
        end
    end

    for source, command in rune.input._commands(input, separator) do
        command = command:match("^%s*(.-)%s*$")
        -- Recognize the prefix before decoding can introduce a literal '#'.
        local count = source:match("^%s*#(%d+)%s+%S")
        local line = count and command:match("^#%d+%s+(.-)%s*$") or command

        for _ = 1, tonumber(count) or 1 do
            if line == "" then
                rune.send_raw(line)
            else
                local processed, result = rune.alias.process(line)
                if not processed then
                    rune.send_raw(line)
                elseif result then
                    send_impl(result, depth + 1) -- alias result is new command text
                end
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
