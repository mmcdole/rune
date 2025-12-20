-- Input bar enhancements
-- Lua is the brain; Go just renders

-- ============================================================
-- Ghost text and Tab completion are handled by 48_completion.lua
-- This file handles other input enhancements
-- ============================================================

-- Escape: clear input
rune.bind("escape", function()
    rune.input.set("")
    rune.input.set_ghost("")
end)

-- ============================================================
-- Word Navigation (logic in Lua, cursor control via Go primitives)
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

-- Keybindings for word navigation
rune.bind("alt+left", function() rune.input.word_left() end)
rune.bind("alt+right", function() rune.input.word_right() end)
rune.bind("ctrl+left", function() rune.input.word_left() end)
rune.bind("ctrl+right", function() rune.input.word_right() end)

-- Keybindings for delete word
rune.bind("alt+backspace", function() rune.input.delete_word() end)
rune.bind("ctrl+backspace", function() rune.input.delete_word() end)

-- ============================================================
-- EDITOR Mode (Ctrl+E opens $EDITOR)
-- ============================================================
rune.bind("ctrl+e", function()
    local current = rune.input.get()
    local result, ok = rune.input.open_editor(current)
    if ok and result ~= "" then
        -- Join multi-line with semicolons
        result = result:gsub("\n", "; ")
        rune.input.set(result)
    end
end)
