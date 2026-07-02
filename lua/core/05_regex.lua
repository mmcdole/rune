-- Regex API
-- Cached pattern matching using Go's regexp engine.
--
-- API:
--   rune.regex.compile(pattern)     -- Compile regex, returns userdata or nil+error
--   rune.regex.validate(pattern)    -- Check a pattern, returns true or nil+error
--   rune.regex.match(pattern, text) -- Match with caching, returns captures array or nil

rune.regex = {}

-- Pattern cache: pattern -> {re = userdata} for valid patterns, or
-- {err = message, reported = bool} for invalid ones. Caching failures
-- means a bad pattern is reported once instead of being recompiled and
-- silently ignored on every line.
local cache = {}

local function lookup(pattern)
    local entry = cache[pattern]
    if not entry then
        local re, err = rune._regex.compile(pattern)
        entry = re and { re = re } or { err = err }
        cache[pattern] = entry
    end
    return entry
end

-- Returns Regex userdata with :match(text) method, or nil + error
function rune.regex.compile(pattern)
    return rune._regex.compile(pattern)
end

-- Validate (and cache) a pattern without matching.
-- Returns true, or nil + error message.
function rune.regex.validate(pattern)
    local entry = lookup(pattern)
    if entry.err then
        return nil, entry.err
    end
    return true
end

-- Match pattern against text, return captures array or nil
-- Caches compiled patterns for performance
function rune.regex.match(pattern, text)
    local entry = lookup(pattern)
    if entry.err then
        if not entry.reported then
            entry.reported = true
            rune.echo(rune.style.red("[Regex]") .. " invalid pattern '" .. tostring(pattern) ..
                "': " .. tostring(entry.err))
        end
        return nil
    end

    -- Match and extract captures (skip index 1 which is full match)
    local matches = entry.re:match(text)
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
