-- Input Field Enhancements
-- Everything that happens WHILE typing (before Enter):
--   - History navigation (Up/Down arrows)
--   - Word navigation (Ctrl/Alt + Left/Right)
--   - Tab completion with word cache

-- ============================================================
-- HISTORY NAVIGATION
-- Implements zsh-style prefix-matching history navigation.
-- Up arrow: search backwards for entries matching current input prefix
-- Down arrow: search forwards, return to draft when exhausted
-- ============================================================

local history_state = {
    index = 0,      -- 0 = at draft, 1..n = position in history (1 = newest)
    draft = "",     -- saved input when entering history
}

local function history_reset()
    history_state.index = 0
    history_state.draft = ""
end

local function history_up()
    local history = rune.history.get()
    if #history == 0 then
        return
    end

    -- If input was externally modified (Ctrl+C, manual edit), reset state
    if history_state.index > 0 then
        local current = rune.input.get()
        local expected = history[#history - history_state.index + 1]
        if current ~= expected then
            history_reset()
        end
    end

    -- Save draft on first navigation
    if history_state.index == 0 then
        history_state.draft = rune.input.get()
    end

    local prefix = history_state.draft

    -- Search backwards from current position
    local start = history_state.index + 1
    if start > #history then
        return -- Already at oldest
    end

    if prefix ~= "" then
        -- Prefix matching: find next entry that starts with prefix
        for i = start, #history do
            local entry = history[#history - i + 1] -- history is oldest-first
            if entry:sub(1, #prefix) == prefix then
                history_state.index = i
                rune.input.set(entry)
                return
            end
        end
        -- No match found, stay where we are
    else
        -- No prefix: cycle through all history
        history_state.index = start
        rune.input.set(history[#history - start + 1])
    end
end

local function history_down()
    if history_state.index == 0 then
        return -- Already at draft
    end

    local history = rune.history.get()

    -- If input was externally modified (Ctrl+C, manual edit), reset state
    local current = rune.input.get()
    local expected = history[#history - history_state.index + 1]
    if current ~= expected then
        history_reset()
        return -- Now at draft, can't go down further
    end

    local prefix = history_state.draft

    if prefix ~= "" then
        -- Prefix matching: find next newer entry that starts with prefix
        for i = history_state.index - 1, 1, -1 do
            local entry = history[#history - i + 1]
            if entry:sub(1, #prefix) == prefix then
                history_state.index = i
                rune.input.set(entry)
                return
            end
        end
        -- No more matches - return to draft
        history_state.index = 0
        rune.input.set(history_state.draft)
    else
        -- No prefix: cycle through history
        if history_state.index == 1 then
            -- Back to draft
            history_state.index = 0
            rune.input.set(history_state.draft)
        else
            history_state.index = history_state.index - 1
            rune.input.set(history[#history - history_state.index + 1])
        end
    end
end

-- History key bindings
rune.bind("up", history_up)
rune.bind("down", history_down)

-- Reset on input submission
rune.hooks.on("input", function(text)
    history_reset()
end, { priority = 1 })

-- Export for potential customization
rune.history_nav = {
    up = history_up,
    down = history_down,
    reset = history_reset,
}

-- ============================================================
-- WORD NAVIGATION & EDITING
-- Logic in Lua, cursor control via Go primitives
-- ============================================================

-- Find previous word boundary
local function find_word_left(text, pos)
    if pos <= 0 then return 0 end
    local newPos = pos
    -- Skip spaces (going backwards)
    while newPos > 0 and text:sub(newPos, newPos) == " " do
        newPos = newPos - 1
    end
    -- Skip word characters
    while newPos > 0 and text:sub(newPos, newPos) ~= " " do
        newPos = newPos - 1
    end
    return newPos
end

-- Find next word boundary
local function find_word_right(text, pos)
    local len = #text
    if pos >= len then return len end
    local newPos = pos + 1
    -- Skip current word characters
    while newPos <= len and text:sub(newPos, newPos) ~= " " do
        newPos = newPos + 1
    end
    -- Skip spaces
    while newPos <= len and text:sub(newPos, newPos) == " " do
        newPos = newPos + 1
    end
    return newPos - 1
end

-- Word left: move cursor to previous word boundary
function rune.input.word_left()
    local text = rune.input.get()
    local pos = rune.input.get_cursor()
    local newPos = find_word_left(text, pos)
    rune.input.set_cursor(newPos)
end

-- Word right: move cursor to next word boundary
function rune.input.word_right()
    local text = rune.input.get()
    local pos = rune.input.get_cursor()
    local newPos = find_word_right(text, pos)
    rune.input.set_cursor(newPos)
end

-- Delete word before cursor
function rune.input.delete_word()
    local text = rune.input.get()
    local pos = rune.input.get_cursor()
    if pos <= 0 then return end

    local newPos = find_word_left(text, pos)
    local before = text:sub(1, newPos)
    local after = text:sub(pos + 1)
    rune.input.set(before .. after)
    rune.input.set_cursor(newPos)
end

-- Escape: clear input
rune.bind("escape", function()
    rune.input.set("")
end)

-- Word navigation keybindings
rune.bind("alt+left", function() rune.input.word_left() end)
rune.bind("alt+right", function() rune.input.word_right() end)
rune.bind("ctrl+left", function() rune.input.word_left() end)
rune.bind("ctrl+right", function() rune.input.word_right() end)

-- Delete word keybindings
rune.bind("alt+backspace", function() rune.input.delete_word() end)
rune.bind("ctrl+backspace", function() rune.input.delete_word() end)

-- Editor mode (Ctrl+E opens $EDITOR)
rune.bind("ctrl+e", function()
    local current = rune.input.get()
    local result, ok = rune.input.open_editor(current)
    if ok and result ~= "" then
        -- Join multi-line with semicolons
        result = result:gsub("\n", "; ")
        rune.input.set(result)
    end
end)

-- ============================================================
-- TAB COMPLETION
-- Word cache from server output + Tab cycling
-- ============================================================

rune.completion = rune.completion or {}

-- Configuration
local MAX_WORDS = 5000
local MIN_WORD_LEN = 3

-- Data structures for word cache
local cache = {}        -- lower -> {word=original, order=int}
local order_list = {}   -- array of lowercase words (insertion order for eviction)
local prefix_idx = {}   -- 2-char -> set of lowercase words
local order_counter = 0

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

-- Completion state (data-driven, no boolean flags)
local completion_state = {
    matches    = {},    -- current matches
    index      = 0,     -- current selection (1-based when cycling)
    word_start = 0,     -- where word starts in input
    word_end   = 0,     -- where word ends in input
    original   = nil,   -- nil = not cycling, string = input before cycling started
    expected   = nil,   -- expected input after Tab (for identity check)
}

-- Export state for status bar to read
rune.completion.state = completion_state

local function completion_reset()
    completion_state.matches = {}
    completion_state.index = 0
    completion_state.word_start = 0
    completion_state.word_end = 0
    completion_state.original = nil
    completion_state.expected = nil
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
    local text = completion_state.original or rune.input.get()
    local before = text:sub(1, completion_state.word_start - 1)
    local after = text:sub(completion_state.word_end + 1)
    local space = after == "" and " " or ""

    local new_input = before .. suggestion .. space .. after

    completion_state.expected = new_input   -- "I expect this..."
    rune.input.set(new_input)               -- "...because I'm setting it"
    rune.input.set_cursor(#before + #suggestion + #space)
end

local function update_ghost()
    local word_start, word_end, prefix = find_word_at_cursor()

    if #prefix < 2 then
        completion_reset()
        return
    end

    local matches = cache_find(prefix)
    if #matches == 0 then
        completion_reset()
        return
    end

    completion_state.matches = matches
    completion_state.index = 1
    completion_state.word_start = word_start
    completion_state.word_end = word_end
end

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
        completion_reset()
        return
    end

    -- 2. Identity check: if this is what Tab just set, ignore
    if completion_state.expected and text == completion_state.expected then
        completion_state.expected = nil
        return
    end

    -- 3. User typed something: exit cycling, update ghost
    completion_state.original = nil
    completion_state.expected = nil
    update_ghost()
end, { name = "_completion_ghost", priority = 100 })

-- Cycle through completions (Tab = forward, Shift+Tab = backward)
local function cycle(direction)
    if #completion_state.matches == 0 then return end

    if not completion_state.original then
        -- First Tab: enter cycling mode
        completion_state.original = rune.input.get()
    else
        -- Subsequent Tabs: advance index
        completion_state.index = ((completion_state.index - 1 + direction) % #completion_state.matches) + 1
    end

    insert_completion(completion_state.matches[completion_state.index])
    rune.ui.refresh_bars()
end

rune.bind("tab", function() cycle(1) end)
rune.bind("shift+tab", function() cycle(-1) end)

-- Public API
function rune.completion.reset()
    completion_reset()
end

function rune.completion.clear_cache()
    cache = {}
    order_list = {}
    prefix_idx = {}
    order_counter = 0
end
