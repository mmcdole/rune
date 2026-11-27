-- Style
-- Colors and attributes for text, and the one place the Lua core writes
-- ANSI escape codes.
--
-- Go colors only its last-resort degraded-mode messages (the text
-- package); all other presentation is Lua's.
--
-- API:
--   rune.style.<name>(text)  -- red green yellow blue magenta cyan white
--                            -- gray, and bold dim inverse
--
-- Notes:
--   Core output conventions: [Error] tags are red (tag only, message
--   plain), [Usage] lines are plain, success tags ([World], [Log]) are
--   green. Name a style local for the style it is: local gray =
--   rune.style.gray.

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
