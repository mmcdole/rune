-- Terminal Styling
-- The one place ANSI escape codes are written out in the Lua core.
-- Presentation belongs to Lua: Go colors only its last-resort
-- degraded-mode messages (see text package).
--
-- Usage: rune.echo(rune.style.red("[Error]") .. " something broke")
--
-- Tag conventions for core output: [Error] tags are red (tag only,
-- message plain), [Usage] lines are plain, success/action tags
-- ([World], [Log], [Loaded], ...) are green.

local function wrap(code)
    local prefix = "\027[" .. code .. "m"
    return function(s)
        return prefix .. tostring(s) .. "\027[0m"
    end
end

rune.style = {
    -- Colors
    red     = wrap(31),
    green   = wrap(32),
    yellow  = wrap(33),
    blue    = wrap(34),
    magenta = wrap(35),
    cyan    = wrap(36),
    white   = wrap(37),
    gray    = wrap(90),

    -- Attributes
    bold    = wrap(1),
    dim     = wrap(2),
    inverse = wrap(7),
}
