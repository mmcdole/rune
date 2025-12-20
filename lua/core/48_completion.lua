-- Word cache and Tab completion
-- Lua is the brain; manages vocabulary from server output

rune.completion = rune.completion or {}

-- ============================================================
-- Word Cache
-- ============================================================

local word_cache = {}      -- lowercase_word -> {word=original_case, order=insertion_order}
local word_list = {}       -- ordered list of lowercase words for LRU
local prefix_index = {}    -- 2-char prefix -> set of lowercase words (for O(1) bucket lookup)
local insertion_counter = 0
local MAX_WORDS = 5000
local MIN_WORD_LEN = 3     -- Skip short words

-- Add word to prefix index (for fast lookup)
local function add_to_index(lower_word)
    if #lower_word < 2 then return end
    local key = lower_word:sub(1, 2)
    prefix_index[key] = prefix_index[key] or {}
    prefix_index[key][lower_word] = true
end

-- Remove word from prefix index
local function remove_from_index(lower_word)
    if #lower_word < 2 then return end
    local key = lower_word:sub(1, 2)
    if prefix_index[key] then
        prefix_index[key][lower_word] = nil
    end
end

-- Strip ANSI escape codes from text
local function strip_ansi(text)
    return text:gsub("\027%[[%d;]*m", "")
end

-- Extract words from a line (preserves original case)
local function extract_words(line)
    local words = {}
    local clean = strip_ansi(line)
    -- Match word characters (letters, numbers, some punctuation)
    for word in clean:gmatch("[%w_'%-]+") do
        if #word >= MIN_WORD_LEN then
            table.insert(words, word)  -- preserve original case
        end
    end
    return words
end

-- Add a word to the cache (LRU), preserving original case
local function add_word(word)
    local lower = word:lower()
    insertion_counter = insertion_counter + 1
    if word_cache[lower] then
        -- Already exists, move to end (LRU) and update case/order
        for i, w in ipairs(word_list) do
            if w == lower then
                table.remove(word_list, i)
                break
            end
        end
        table.insert(word_list, lower)
        word_cache[lower] = { word = word, order = insertion_counter }
    else
        -- New word
        word_cache[lower] = { word = word, order = insertion_counter }
        table.insert(word_list, lower)
        add_to_index(lower)
        -- Trim if over capacity
        while #word_list > MAX_WORDS do
            local oldest = table.remove(word_list, 1)
            word_cache[oldest] = nil
            remove_from_index(oldest)
        end
    end
end

-- Add words from server output
rune.hooks.on("output", function(line)
    -- line is a Line userdata with :raw() and :clean() methods
    local words = extract_words(line:clean())
    for _, word in ipairs(words) do
        add_word(word)
    end
end, { name = "_word_cache", priority = 200 })

-- Also add words from user input (when submitted)
rune.hooks.on("input", function(text)
    local words = extract_words(text)
    for _, word in ipairs(words) do
        add_word(word)
    end
end, { name = "_word_cache_input", priority = 200 })

-- ============================================================
-- Word Boundary Detection
-- ============================================================

-- Find word prefix before cursor position (for completion)
-- Returns: word_start (1-indexed), word_end (1-indexed), prefix
local function find_word_boundaries(text, cursor)
    if text == "" or cursor <= 0 then
        return 1, 0, ""
    end

    -- Lua strings are 1-indexed, cursor from Go is 0-indexed
    -- cursor=3 means insertion point is after character 3
    local pos = cursor

    -- Find word start (scan backwards from cursor)
    local word_start = pos
    while word_start > 0 and text:sub(word_start, word_start):match("[%w_'%-]") do
        word_start = word_start - 1
    end
    word_start = word_start + 1  -- Move forward to first word char

    -- Word end is at cursor position (we only care about prefix for completion)
    local word_end = pos

    if word_start > word_end then
        return word_start, word_start - 1, ""
    end

    local prefix = text:sub(word_start, word_end)
    return word_start, word_end, prefix
end

-- ============================================================
-- Completion Logic
-- ============================================================

-- Find matches for a prefix (returns newest first, with original case)
local function find_matches(prefix)
    if prefix == "" or #prefix < 2 then return {} end
    local lower_prefix = prefix:lower()
    local key = lower_prefix:sub(1, 2)
    local bucket = prefix_index[key]
    if not bucket then return {} end

    -- Collect matching words from bucket
    local candidates = {}
    for lower_word in pairs(bucket) do
        if lower_word:sub(1, #lower_prefix) == lower_prefix and lower_word ~= lower_prefix then
            table.insert(candidates, lower_word)
        end
    end

    -- Sort by recency using stored order (higher = newer)
    table.sort(candidates, function(a, b)
        local ca, cb = word_cache[a], word_cache[b]
        return (ca and ca.order or 0) > (cb and cb.order or 0)
    end)

    -- Return top 10 with original case
    local matches = {}
    for i = 1, math.min(10, #candidates) do
        local entry = word_cache[candidates[i]]
        if entry then
            table.insert(matches, entry.word)
        end
    end
    return matches
end

-- Current ghost/completion state
local ghost_state = {
    suggestion = "",      -- The full suggested word
    word_start = 0,       -- Where the current word starts in the input
    word_end = 0,         -- Where the current word ends
    prefix = "",          -- What the user typed (the prefix being completed)
}

-- Reset ghost state
local function reset_ghost()
    ghost_state.suggestion = ""
    ghost_state.word_start = 0
    ghost_state.word_end = 0
    ghost_state.prefix = ""
    rune.input.set_ghost("")
end

-- Update ghost suggestion based on current input
local function update_ghost()
    local text = rune.input.get()
    local cursor = rune.input.get_cursor()

    -- Find word at cursor
    local word_start, word_end, current_word = find_word_boundaries(text, cursor)

    -- Need at least 2 chars to suggest
    if current_word == "" or #current_word < 2 then
        reset_ghost()
        return
    end

    -- Find best match
    local matches = find_matches(current_word)
    if #matches == 0 then
        reset_ghost()
        return
    end

    local suggestion = matches[1]  -- Best/newest match

    -- Store state
    ghost_state.suggestion = suggestion
    ghost_state.word_start = word_start
    ghost_state.word_end = word_end
    ghost_state.prefix = current_word

    -- Build ghost text: preserve user's typed text + add completion remainder
    -- This ensures Go's prefix check passes (case-sensitive)
    local completion_remainder = suggestion:sub(#current_word + 1)
    local ghost_text = text .. completion_remainder

    rune.input.set_ghost(ghost_text)
end

-- Accept the current ghost suggestion
local function accept_ghost()
    if ghost_state.suggestion == "" then
        return false
    end

    local text = rune.input.get()
    local before = text:sub(1, ghost_state.word_start - 1)
    local after = text:sub(ghost_state.word_end + 1)

    -- Add space after if at end
    local smart_space = ""
    if after == "" then
        smart_space = " "
    end

    local new_text = before .. ghost_state.suggestion .. smart_space .. after
    local new_cursor = #before + #ghost_state.suggestion + #smart_space

    rune.input.set(new_text)
    rune.input.set_cursor(new_cursor)
    reset_ghost()

    return true
end

-- Tab: accept ghost suggestion
rune.bind("tab", function()
    if ghost_state.suggestion ~= "" then
        accept_ghost()
    end
end)

-- Update ghost on input change
rune.hooks.on("input_changed", function(text)
    update_ghost()
end, { name = "_completion_ghost", priority = 100 })

-- Export for potential use
function rune.completion.complete_word()
    accept_ghost()
end
