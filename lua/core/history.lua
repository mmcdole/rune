-- History
-- Command history: the public list, Up/Down prefix navigation, and the
-- Ctrl+R picker.
--
-- Go stores history (rune._history), so it survives /reload, and records
-- each submission; this file owns navigation.
--
-- API:
--   rune.history.get()      -- entry texts, oldest first
--   rune.history.add(text)  -- record a command without running it
--
-- Notes:
--   Up searches backwards for entries starting with what is typed (zsh
--   style); Down returns toward the draft. "!" history expansion lives in
--   input.lua, where command syntax is parsed.

rune.history = {}

-- Return history text oldest-first, including multiline and Verbatim entries.
-- Mode metadata stays in Go; this public API returns only the text.
function rune.history.get()
    local entries = rune._history.entries()
    local history = {}
    for i, entry in ipairs(entries) do
        history[i] = entry.text
    end
    return history
end

-- Record Command-mode text without sending it or running input processing.
-- Go validates the text and applies the normal history storage rules.
function rune.history.add(cmd)
    rune._history.add(cmd)
end

-- Navigation

local history_state = {
    index = 0,      -- 0 = at draft, 1..n = position in history (1 = newest)
    draft = { text = "", mode = "command" },
}

local function command_entry(text)
    return { text = text, mode = "command" }
end

local function restore_history_entry(entry)
    rune._input.restore(entry.text, entry.mode)
end

local function history_reset()
    history_state.index = 0
    history_state.draft = command_entry("")
end

-- Entries are found in Go one at a time (rune._history.find), so Up and
-- Down never copy the whole history into Lua.

-- The input still shows the entry at the current position: not edited,
-- cleared, or trimmed out of history since it was restored.
local function at_current_entry()
    local pos, text = rune._history.find("", history_state.index, 1)
    return pos ~= nil and rune.input.get() == text
end

local function history_up()
    -- If input was externally modified (Ctrl+C, manual edit), reset state
    if history_state.index > 0 and not at_current_entry() then
        history_reset()
    end

    -- Save draft on first navigation
    if history_state.index == 0 then
        history_state.draft = command_entry(rune.input.get())
    end

    -- Search backwards from the current position for the next entry that
    -- starts with the draft (an empty draft matches every entry). No match,
    -- or already at the oldest: stay where we are.
    local pos, text, mode = rune._history.find(
        history_state.draft.text, history_state.index + 1, 1)
    if pos then
        history_state.index = pos
        rune._input.restore(text, mode)
    end
end

local function history_down()
    if history_state.index == 0 then
        return -- Already at draft
    end

    -- If input was externally modified (Ctrl+C, manual edit), reset state
    if not at_current_entry() then
        history_reset()
        return -- Now at draft, can't go down further
    end

    -- Search forwards for the next newer entry that starts with the draft
    -- (an empty draft matches every entry). None left: back to the draft.
    local pos, text, mode = rune._history.find(
        history_state.draft.text, history_state.index - 1, -1)
    if pos then
        history_state.index = pos
        rune._input.restore(text, mode)
    else
        history_state.index = 0
        restore_history_entry(history_state.draft)
    end
end

-- Defaults

-- History key bindings
rune.bind("up", history_up)
rune.bind("down", history_down)

-- History search (Ctrl+R). Keeping selection beside the navigation state
-- lets an entry chosen from the picker continue naturally with Up/Down and
-- return to the draft that was present before the picker opened.
rune.bind("ctrl+r", function()
    local history = rune._history.entries()
    local draft = rune.input.get()

    -- Reverse history for display (newest first).
    local items = {}
    for i = #history, 1, -1 do
        local entry = history[i]
        items[#items + 1] = {
            text = entry.text,
            desc = entry.mode == "verbatim" and "verbatim" or "",
            value = tostring(i),
        }
    end

    rune.ui.picker.show({
        title = "History",
        items = items,
        on_select = function(index)
            local history_index = tonumber(index)
            local entry = history[history_index]
            if entry then
                history_state.index = #history - history_index + 1
                history_state.draft = command_entry(draft)
                restore_history_entry(entry)
            end
        end
    })
end)

-- Submission is a Go action (input.submit), so the input event is how
-- Lua learns of it. Priority 1 runs before any user hook can consume it.
rune.hooks.on("input", function()
    history_reset()
end, { name = "_history-reset", priority = 1 })
