-- Completion
-- Tab completion from words seen in server output and in your own commands,
-- plus the "/" command picker and the Ctrl+T alias picker.
--
-- Pure Lua; the input widget is Go's (rune._input). The status bar
-- (statusbar.lua) shows the match list from rune.completion.state.
--
-- API (internal):
--   rune.completion.state   -- matches, index, and cycling state
--
-- Notes:
--   Remembers up to 5000 words of three or more letters, newest first.
--   Lines typed while the server hides input (passwords) are never read.

rune.completion = {}

-- Configuration
local MAX_WORDS = 5000
local MIN_WORD_LEN = 3

-- Data structures for word cache
local cache = {}      -- lower -> {word=original, order=int}
-- Fixed-size ring of lowercase words in insertion order: the slot a
-- new word claims holds the eviction victim, so eviction is O(1).
local ring = {}
local ring_pos = 0    -- last written slot (1..MAX_WORDS)
local prefix_idx = {} -- 2-char -> set of lowercase words
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
        return
    end

    -- New word: claim the next ring slot, evicting its occupant.
    ring_pos = ring_pos % MAX_WORDS + 1
    local evicted = ring[ring_pos]
    if evicted then
        cache[evicted] = nil
        local bucket = prefix_idx[evicted:sub(1, 2)]
        if bucket then
            bucket[evicted] = nil
        end
    end
    ring[ring_pos] = lower

    cache[lower] = { word = word, order = order_counter }
    local key = lower:sub(1, 2)
    prefix_idx[key] = prefix_idx[key] or {}
    prefix_idx[key][lower] = true
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

local function update_matches()
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

-- Defaults

-- Add words from server output
rune.hooks.on("output", function(line)
    for word in line:clean():gmatch("[%w_'%-]+") do
        cache_add(word)
    end
end, { name = "_completion-output", priority = 200 })

-- Add words from the effective input after core history expansion.
rune.hooks.on("input", function(text, context)
    if context.hidden then
        return -- never remember a password
    end
    for word in text:gmatch("[%w_'%-]+") do
        cache_add(word)
    end
end, { name = "_completion-input", priority = 200 })

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

    -- 3. User typed something: exit cycling, refresh matches
    completion_state.original = nil
    completion_state.expected = nil
    update_matches()
end, { name = "_completion-matches", priority = 100 })

rune.bind("tab", function() cycle(1) end)
rune.bind("shift+tab", function() cycle(-1) end)

-- Alias Search (Ctrl+T)
rune.bind("ctrl+t", function()
    local aliases = rune.alias.list()

    -- Format for picker
    local items = {}
    for _, a in ipairs(aliases) do
        items[#items + 1] = {
            text = a.match,
            desc = a.value,
            value = a.match,
        }
    end

    rune.ui.picker.show({
        title = "Aliases",
        items = items,
        on_select = function(val)
            rune.input.set(val)
        end
    })
end)


-- Slash Command Picker (Inline Mode)
-- Opens a picker that filters as you type after "/"
rune.bind("/", function()
    -- Set input to "/" so user sees what they're typing
    rune.input.set("/")

    -- Get all available commands
    local cmds = rune.command.list()

    -- Format for picker (include "/" in text/value for matching)
    local items = {}
    for _, c in ipairs(cmds) do
        items[#items + 1] = {
            text = "/" .. c.name,
            desc = c.description,
            value = "/" .. c.name,
        }
    end

    -- Open picker in inline mode
    -- The picker filters based on full input content; once the user
    -- commits to a command and types a space to start its arguments,
    -- filtering command names is pure noise, so the picker dismisses.
    rune.ui.picker.show({
        items = items,
        mode = "inline",
        match_description = true,
        dismiss_on_space = true,
        on_select = function(val)
            -- Selection completes - the UI already set input to "/command "
        end
    })
end)
