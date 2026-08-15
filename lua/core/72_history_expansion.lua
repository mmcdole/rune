-- Interactive history expansion.
--
-- This is an input transform, not an alias: successful expansion becomes the
-- value committed to history and later dispatched, while programmatic
-- rune.send calls remain independent of interactive submission history.

local function repeat_spec(piece)
    local spec = piece:match("^%s*!([^%s]*)%s*$")
    if spec == "!" then
        return "" -- `!!` is equivalent to bare `!`.
    end
    return spec
end

-- History can contain entries created by older versions or explicitly through
-- rune.history.add. Never expand into another designator: a compound entry
-- such as `north;!` would otherwise feed history syntax back into dispatch.
local function contains_repeat(text, delimiter)
    if not text:find("!", 1, true) then
        return false
    end

    local start = 1
    while true do
        local pos = text:find(delimiter, start, true)
        local piece = pos and text:sub(start, pos - 1) or text:sub(start)
        if repeat_spec(piece) ~= nil then
            return true
        end
        if not pos then
            return false
        end
        start = pos + #delimiter
    end
end

local function find_previous(spec, history, delimiter)
    for i = #history, 1, -1 do
        local entry = history[i]
        if entry.mode == "command" then
            local candidate = entry.text:match("^%s*(.-)%s*$")
            if candidate:sub(1, 1) ~= "/"
                and not contains_repeat(entry.text, delimiter)
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
    -- Dispatch treats a leading slash as one local command before delimiter
    -- processing. Keep its arguments literal for the same reason slash
    -- commands are never eligible history candidates.
    if text:sub(1, 1) == "/" then
        return nil
    end
    if not text:find("!", 1, true) then
        return nil
    end

    local delimiter = rune.config.get("delimiter")
    if not contains_repeat(text, delimiter) then
        return nil
    end
    local history = rune._history.entries()
    local pieces = {}
    local changed = false
    local start = 1

    while true do
        local pos = text:find(delimiter, start, true)
        local piece = pos and text:sub(start, pos - 1) or text:sub(start)
        local spec = repeat_spec(piece)

        if spec ~= nil then
            local replacement = find_previous(spec, history, delimiter)
            if not replacement then
                local token = piece:match("^%s*(.-)%s*$")
                rune.echo(rune.style.yellow("[History]") ..
                    " no matching command: " .. token)
                return false
            end
            pieces[#pieces + 1] = replacement
            changed = true
        else
            pieces[#pieces + 1] = piece
        end

        if not pos then
            break
        end
        start = pos + #delimiter
    end

    if changed then
        return table.concat(pieces, delimiter)
    end
end

rune.hooks.on("input", expand_history, {
    name = "history-expansion",
    priority = 100,
})
