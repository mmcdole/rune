-- History Navigation
-- Implements zsh-style prefix-matching history navigation.
-- Up arrow: search backwards for entries matching current input prefix
-- Down arrow: search forwards, return to draft when exhausted

local state = {
    index = 0,      -- 0 = at draft, 1..n = position in history (1 = newest)
    draft = "",     -- saved input when entering history
}

-- Navigate up in history (older entries)
local function history_up()
    local history = rune.history.get()
    if #history == 0 then
        return
    end

    -- Save draft on first navigation
    if state.index == 0 then
        state.draft = rune.input.get()
    end

    local prefix = state.draft

    -- Search backwards from current position
    local start = state.index + 1
    if start > #history then
        return -- Already at oldest
    end

    if prefix ~= "" then
        -- Prefix matching: find next entry that starts with prefix
        for i = start, #history do
            local entry = history[#history - i + 1] -- history is oldest-first
            if entry:sub(1, #prefix) == prefix then
                state.index = i
                rune.input.set(entry)
                return
            end
        end
        -- No match found, stay where we are
    else
        -- No prefix: cycle through all history
        state.index = start
        rune.input.set(history[#history - start + 1])
    end
end

-- Navigate down in history (newer entries)
local function history_down()
    if state.index == 0 then
        return -- Already at draft
    end

    local history = rune.history.get()
    local prefix = state.draft

    if prefix ~= "" then
        -- Prefix matching: find next newer entry that starts with prefix
        for i = state.index - 1, 1, -1 do
            local entry = history[#history - i + 1]
            if entry:sub(1, #prefix) == prefix then
                state.index = i
                rune.input.set(entry)
                return
            end
        end
        -- No more matches - return to draft
        state.index = 0
        rune.input.set(state.draft)
    else
        -- No prefix: cycle through history
        if state.index == 1 then
            -- Back to draft
            state.index = 0
            rune.input.set(state.draft)
        else
            state.index = state.index - 1
            rune.input.set(history[#history - state.index + 1])
        end
    end
end

-- Reset navigation state
local function reset()
    state.index = 0
    state.draft = ""
end

-- Bind arrow keys
rune.bind("up", history_up)
rune.bind("down", history_down)

-- Reset on input submission
rune.hooks.register("input", function(text)
    reset()
end, { priority = 1 })

-- Export for potential customization
rune.history_nav = {
    up = history_up,
    down = history_down,
    reset = reset,
}
