-- Default Status Bar Implementation
-- Uses the reactive rune.ui.bar() API to render status based on rune.state

-- ANSI color helpers
local function green(s) return "\027[32m" .. s .. "\027[0m" end
local function red(s) return "\027[31m" .. s .. "\027[0m" end
local function yellow(s) return "\027[33m" .. s .. "\027[0m" end
local function gray(s) return "\027[90m" .. s .. "\027[0m" end
local function dim(s) return "\027[2m" .. s .. "\027[0m" end

-- Ctrl+C double-tap quit state
local quit_pending = false

-- Ctrl+C binding: first press shows warning, second press quits
rune.bind("ctrl+c", function()
    if quit_pending then
        rune.quit()
    else
        quit_pending = true
        rune.timer.after(2, function()
            quit_pending = false
        end, {name = "_quit_timeout"})
    end
end)

-- Register the status bar renderer
-- This function is called by Session every 250ms to get current bar content
rune.ui.bar("status", function(width)
    local state = rune.state

    -- Left side: connection status (or quit warning)
    local left
    if quit_pending then
        left = yellow("Press Ctrl+C again to exit")
    elseif state.connected then
        left = green("●") .. " " .. gray(state.address)
    else
        left = gray("●") .. " " .. gray("Disconnected")
    end

    -- Right side: scroll mode indicator
    local right
    if state.scroll_mode == "scrolled" then
        right = yellow("SCROLL") .. " " .. dim("(" .. state.scroll_lines .. " new)")
    else
        right = dim("LIVE")
    end

    return { left = left, right = right }
end)

-- Set default layout: input line with status bar below
-- This can be overridden by user's init.lua
rune.ui.layout({
    bottom = { "input", "status" }
})
