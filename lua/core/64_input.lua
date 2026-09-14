-- Input processing.
-- Session calls _prepare (history expansion, then input hooks), echoes the
-- result, then calls _dispatch. Session owns physical lines and history recording.
-- Programmatic rune.send executes game commands without interactive preparation.

-- Shared command boundaries for sending and history expansion. Keep the source
-- spelling alongside the decoded command so history can safely replay escapes.
local function commands(text, separator)
    local start = 1
    local doubled = separator .. separator
    local escaped_pattern = doubled:gsub("([^%w])", "%%%1")

    return function()
        if not start then return nil end

        local cursor = start
        while true do
            local pos = text:find(separator, cursor, true)
            if pos and text:sub(pos, pos + #doubled - 1) == doubled then
                cursor = pos + #doubled
            else
                local source = pos and text:sub(start, pos - 1) or text:sub(start)
                start = pos and pos + #separator or nil
                local command = source:gsub(escaped_pattern, function() return separator end)
                return source, command
            end
        end
    end
end

-- Interactive history expansion.
--
-- Called once before input hooks. Programmatic sends and hook rewrites never
-- re-enter expansion. Keep source spelling intact for echo, history, and replay.

local function designator_spec(piece, marker, separator)
    local token = piece:match("^%s*(.-)%s*$")
    if token:sub(1, #separator) == separator or token:sub(1, #marker) ~= marker then
        return nil
    end

    local spec = token:sub(#marker + 1)
    if spec:find("%s") then
        return nil
    end
    if spec == marker then
        return "" -- A doubled marker is equivalent to a bare marker.
    end
    return spec
end

-- History-expansion syntax can enter history through rune.history.add or while
-- expansion uses a different character. Skip those entries rather than send
-- unresolved expansion text to the MUD.
local function contains_designator(text, separator, marker)
    if not text:find(marker, 1, true) then
        return false
    end

    for piece in commands(text, separator) do
        if designator_spec(piece, marker, separator) ~= nil then
            return true
        end
    end
    return false
end

local function find_previous(spec, history, separator, marker)
    for i = #history, 1, -1 do
        local entry = history[i]
        if entry.mode == "command" then
            -- A history entry can hold a batch. Expansion selects its most
            -- recent eligible command line; history navigation recalls the block.
            local lines = {}
            local normalized = entry.text:gsub("\r\n", "\n"):gsub("\r", "\n")
            for line in (normalized .. "\n"):gmatch("(.-)\n") do
                lines[#lines + 1] = line
            end
            for j = #lines, 1, -1 do
                local candidate = lines[j]:match("^%s*(.-)%s*$")
                if candidate ~= ""
                    and lines[j]:sub(1, 1) ~= "/"
                    and not contains_designator(lines[j], separator, marker)
                    and (spec == "" or candidate:sub(1, #spec) == spec)
                then
                    return lines[j]
                end
            end
        end
    end
end

local function expand_commands(text, history, separator, marker)
    local pieces = {}
    for piece in commands(text, separator) do
        local spec = designator_spec(piece, marker, separator)
        if spec ~= nil then
            local replacement = find_previous(spec, history, separator, marker)
            if not replacement then
                local token = piece:match("^%s*(.-)%s*$")
                rune.echo(rune.style.yellow("[History]") ..
                    " no matching command: " .. token)
                return false
            end
            pieces[#pieces + 1] = replacement
        else
            pieces[#pieces + 1] = piece
        end
    end

    -- A recalled line may start or end with separators. Keep them from
    -- pairing with the separator that joins it to the surrounding commands.
    for i = 2, #pieces do
        if pieces[i - 1]:sub(-#separator) == separator
            or pieces[i]:sub(1, #separator) == separator
        then
            pieces[i - 1] = pieces[i - 1] .. " "
            pieces[i] = " " .. pieces[i]
        end
    end
    return table.concat(pieces, separator)
end

-- Returns resolved text, or false when a reference has no match.
local function expand_history(text)
    if text:sub(1, 1) == "/" then return text end
    local marker = rune.config.get("history_character")
    if marker == "" or not text:find(marker, 1, true) then return text end

    local separator = rune.config.get("command_separator")
    if contains_designator(text, separator, marker) then
        return expand_commands(text, rune._history.entries(), separator, marker)
    end
    return text
end

-- Command sequences, single-command repeats, and alias expansion.

local MAX_RECURSION_DEPTH = 100

local function execute_commands(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
        return
    end

    local separator = rune.config.get("command_separator")
    for source, command in commands(input, separator) do
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

-- History resolves exactly once, before any user input handler.
function rune.input._prepare(text, mode)
    if mode == "command" then
        text = expand_history(text)
        if text == false then return false end
    end
    -- A missing hook system still permits recovery commands and literal sends.
    if type(rune.hooks) ~= "table" or type(rune.hooks.call) ~= "function" then
        return text
    end
    return rune.hooks.call("input", text, { mode = mode })
end

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
