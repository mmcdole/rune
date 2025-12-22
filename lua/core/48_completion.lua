-- Word cache and Tab completion
-- Lua is the brain; manages vocabulary from server output

rune.completion = rune.completion or {}

-- Configuration
local MAX_WORDS = 5000
local MIN_WORD_LEN = 3

-- Data structures for word cache
local cache = {}        -- lower -> {word=original, order=int}
local order_list = {}   -- array of lowercase words (insertion order for eviction)
local prefix_idx = {}   -- 2-char -> set of lowercase words
local order_counter = 0

-- ============================================================
-- Word Cache Operations
-- ============================================================

local function cache_add(word)
    local lower = word:lower()
    if #lower < MIN_WORD_LEN then return end

    order_counter = order_counter + 1
    local entry = cache[lower]

    if entry then
        -- Update existing entry (bump recency)
        entry.word = word
        entry.order = order_counter
    else
        -- New word
        cache[lower] = { word = word, order = order_counter }
        order_list[#order_list + 1] = lower

        -- Add to prefix index
        if #lower >= 2 then
            local key = lower:sub(1, 2)
            prefix_idx[key] = prefix_idx[key] or {}
            prefix_idx[key][lower] = true
        end

        -- Evict oldest if over capacity
        if #order_list > MAX_WORDS then
            local oldest = table.remove(order_list, 1)
            local old_key = oldest:sub(1, 2)
            if prefix_idx[old_key] then
                prefix_idx[old_key][oldest] = nil
            end
            cache[oldest] = nil
        end
    end
end

local function cache_find(prefix)
    if #prefix < 2 then return {} end

    local lower_prefix = prefix:lower()
    local bucket = prefix_idx[lower_prefix:sub(1, 2)]
    if not bucket then return {} end

    -- Collect matching words from bucket
    local matches = {}
    for lower_word in pairs(bucket) do
        if lower_word:sub(1, #lower_prefix) == lower_prefix
           and lower_word ~= lower_prefix then
            local entry = cache[lower_word]
            if entry then
                matches[#matches + 1] = { word = entry.word, order = entry.order }
            end
        end
    end

    -- Sort by recency (higher order = newer)
    table.sort(matches, function(a, b) return a.order > b.order end)

    -- Return top 10 words
    local result = {}
    for i = 1, math.min(10, #matches) do
        result[i] = matches[i].word
    end
    return result
end

-- ============================================================
-- Completion State (data-driven, no boolean flags)
-- ============================================================

local state = {
    matches    = {},    -- current matches
    index      = 0,     -- current selection (1-based when cycling)
    word_start = 0,     -- where word starts in input
    word_end   = 0,     -- where word ends in input
    original   = nil,   -- nil = not cycling, string = input before cycling started
    expected   = nil,   -- expected input after Tab (for identity check)
}

-- Export state for status bar to read
rune.completion.state = state

local function reset_state()
    state.matches = {}
    state.index = 0
    state.word_start = 0
    state.word_end = 0
    state.original = nil
    state.expected = nil
end

local function find_word_at_cursor()
    local text = rune.input.get()
    local cursor = rune.input.get_cursor()
    if text == "" or cursor <= 0 then
        return 0, 0, ""
    end

    -- Find word start (scan backwards from cursor)
    local word_start = cursor
    while word_start > 0 and text:sub(word_start, word_start):match("[%w_'%-]") do
        word_start = word_start - 1
    end
    word_start = word_start + 1

    if word_start > cursor then
        return 0, 0, ""
    end

    return word_start, cursor, text:sub(word_start, cursor)
end

local function insert_completion(suggestion)
    local text = state.original or rune.input.get()
    local before = text:sub(1, state.word_start - 1)
    local after = text:sub(state.word_end + 1)
    local space = after == "" and " " or ""

    local new_input = before .. suggestion .. space .. after

    state.expected = new_input   -- "I expect this..."
    rune.input.set(new_input)    -- "...because I'm setting it"
    rune.input.set_cursor(#before + #suggestion + #space)
end

local function update_ghost()
    local word_start, word_end, prefix = find_word_at_cursor()

    if #prefix < 2 then
        reset_state()
        return
    end

    local matches = cache_find(prefix)
    if #matches == 0 then
        reset_state()
        return
    end

    state.matches = matches
    state.index = 1
    state.word_start = word_start
    state.word_end = word_end
end

-- ============================================================
-- Hooks
-- ============================================================

-- Add words from server output
rune.hooks.on("output", function(line)
    for word in line:clean():gmatch("[%w_'%-]+") do
        cache_add(word)
    end
end, { name = "_completion_cache", priority = 200 })

-- Add words from user input
rune.hooks.on("input", function(text)
    for word in text:gmatch("[%w_'%-]+") do
        cache_add(word)
    end
end, { name = "_completion_input", priority = 200 })

-- Smart input_changed hook: data-driven, no flags
rune.hooks.on("input_changed", function()
    local text = rune.input.get()

    -- 1. Empty input: always reset
    if text == "" then
        reset_state()
        return
    end

    -- 2. Identity check: if this is what Tab just set, ignore
    if state.expected and text == state.expected then
        state.expected = nil
        return
    end

    -- 3. User typed something: exit cycling, update ghost
    state.original = nil
    state.expected = nil
    update_ghost()
end, { name = "_completion_ghost", priority = 100 })

-- ============================================================
-- Key Bindings
-- ============================================================

-- Cycle through completions (Tab = forward, Shift+Tab = backward)
local function cycle(direction)
    if #state.matches == 0 then return end

    if not state.original then
        -- First Tab: enter cycling mode
        state.original = rune.input.get()
    else
        -- Subsequent Tabs: advance index
        state.index = ((state.index - 1 + direction) % #state.matches) + 1
    end

    insert_completion(state.matches[state.index])
    rune.ui.refresh_bars()
end

rune.bind("tab", function() cycle(1) end)
rune.bind("shift+tab", function() cycle(-1) end)

-- ============================================================
-- Public API
-- ============================================================

function rune.completion.reset()
    reset_state()
end

function rune.completion.clear_cache()
    cache = {}
    order_list = {}
    prefix_idx = {}
    order_counter = 0
end
