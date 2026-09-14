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

    for piece in rune.input._commands(text, separator) do
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
    for piece in rune.input._commands(text, separator) do
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
function rune.input._expand_history(text)
    if text:sub(1, 1) == "/" then return text end
    local marker = rune.config.get("history_character")
    if marker == "" or not text:find(marker, 1, true) then return text end

    local separator = rune.config.get("command_separator")
    if contains_designator(text, separator, marker) then
        return expand_commands(text, rune._history.entries(), separator, marker)
    end
    return text
end
