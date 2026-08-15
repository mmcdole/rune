-- Interactive history expansion.
--
-- This is an input transform, not an alias: successful expansion becomes the
-- text stored in history and processed as input. Programmatic rune.send calls
-- do not use interactive submission history.

local function designator_spec(piece, marker)
    local token = piece:match("^%s*(.-)%s*$")
    if token:sub(1, #marker) ~= marker then
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

    local start = 1
    while true do
        local pos = text:find(separator, start, true)
        local piece = pos and text:sub(start, pos - 1) or text:sub(start)
        if designator_spec(piece, marker) ~= nil then
            return true
        end
        if not pos then
            return false
        end
        start = pos + #separator
    end
end

local function find_previous(spec, history, separator, marker)
    for i = #history, 1, -1 do
        local entry = history[i]
        if entry.mode == "command" then
            local candidate = entry.text:match("^%s*(.-)%s*$")
            if candidate ~= ""
                and entry.text:sub(1, 1) ~= "/"
                and not contains_designator(entry.text, separator, marker)
                and (spec == "" or candidate:sub(1, #spec) == spec)
            then
                return entry.text
            end
        end
    end
end

local function expand_history(text, context)
    if context.mode ~= "command" then
        return nil
    end
    -- A leading slash is a local Rune command; leave its whole line literal.
    if text:sub(1, 1) == "/" then
        return nil
    end

    local marker = rune.config.get("history_character")
    if marker == "" or not text:find(marker, 1, true) then
        return nil
    end

    local separator = rune.config.get("command_separator")
    if not contains_designator(text, separator, marker) then
        return nil
    end
    local history = rune._history.entries()
    local pieces = {}
    local start = 1

    while true do
        local pos = text:find(separator, start, true)
        local piece = pos and text:sub(start, pos - 1) or text:sub(start)
        local spec = designator_spec(piece, marker)

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

        if not pos then
            break
        end
        start = pos + #separator
    end

    return table.concat(pieces, separator)
end

rune.hooks.on("input", expand_history, {
    name = "history-expansion",
    priority = 100,
})
