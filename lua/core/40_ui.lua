-- Default Status Bar Implementation
-- Uses the reactive rune.ui.bar() API to render status based on rune.state

-- ANSI color helpers
local function green(s) return "\027[32m" .. s .. "\027[0m" end
local function yellow(s) return "\027[33m" .. s .. "\027[0m" end
local function gray(s) return "\027[90m" .. s .. "\027[0m" end
local function dim(s) return "\027[2m" .. s .. "\027[0m" end
local function inverse(s) return "\027[7m" .. s .. "\027[0m" end

-- Render completion bar when cycling through matches
local function render_completion_bar(width)
    local comp = rune.completion.state
    local parts = {}

    for i, match in ipairs(comp.matches) do
        if i == comp.index then
            -- Current selection: inverse (highlighted)
            parts[#parts + 1] = inverse(match)
        else
            -- Other options: dimmed
            parts[#parts + 1] = dim(match)
        end
    end

    local left = table.concat(parts, " ")
    local right = dim("(" .. comp.index .. "/" .. #comp.matches .. ")")

    return { left = left, right = right }
end

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
    -- Check if we should show completion bar instead (only when cycling with multiple matches)
    local comp = rune.completion and rune.completion.state
    if comp and comp.original and #comp.matches > 1 then
        return render_completion_bar(width)
    end

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
