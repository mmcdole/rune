-- Command sequences, single-command repeats, and alias expansion.

local MAX_RECURSION_DEPTH = 100

local function execute_commands(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
        return
    end

    local separator = rune.config.get("command_separator")
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
                    execute_commands(result, depth + 1) -- alias result is new command text
                end
            end
        end
    end
end

-- PUBLIC: Execute game-command syntax and aliases, then send.
-- Interactive hooks, echo, history, and slash commands belong above this entry.
function rune.send(input)
    execute_commands(input, 0)
end
