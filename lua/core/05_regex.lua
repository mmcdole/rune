-- Regex API
-- Cached pattern matching using Go's regexp engine.
--
-- API:
--   rune.regex.compile(pattern)     -- Compile regex, returns userdata or nil+error
--   rune.regex.match(pattern, text) -- Match with caching, returns captures array or nil

rune.regex = {}

-- Pattern cache for compiled regexes
local regex_cache = {}

-- Returns Regex userdata with :match(text) method, or nil + error
function rune.regex.compile(pattern)
    return rune._regex.compile(pattern)
end

-- Match pattern against text, return captures array or nil
-- Caches compiled patterns for performance
function rune.regex.match(pattern, text)
    -- Get or compile regex
    local re = regex_cache[pattern]
    if not re then
        local err
        re, err = rune._regex.compile(pattern)
        if not re then
            return nil
        end
        regex_cache[pattern] = re
    end

    -- Match and extract captures (skip index 1 which is full match)
    local matches = re:match(text)
    if not matches then
        return nil
    end

    -- Return captures only (index 2+), or empty table if no captures
    local captures = {}
    for i = 2, #matches do
        captures[i - 1] = matches[i]
    end
    return captures
end
