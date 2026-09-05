-- Shared command boundaries for sending and history expansion. Keep the source
-- spelling alongside the decoded command so history can safely replay escapes.
function rune.input._commands(text, separator)
    local start = 1
    local doubled = separator .. separator
    local escaped_pattern = doubled:gsub("([^%w])", "%%%1")

    return function()
        if not start then return nil end

        -- A braced repeat is one command; its body is parsed when expanded.
        local group = text:sub(start):match("^%s*#%d+%s*{[^}]+}")
        local cursor = start + (group and #group or 0)
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
