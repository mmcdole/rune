-- Status bar
-- Rune's default status bar (connection, scroll state, the Tab completion
-- list) and the Ctrl+C double-press quit it announces.
--
-- Go calls the bar renderer on its render tick (bars.lua); this file owns
-- what the bar shows and the quit state the Ctrl+C key and the bar share.
--
-- Notes:
--   Replace the bar by registering your own: rune.ui.bar("status", fn).

local green, yellow, gray, dim, inverse =
    rune.style.green, rune.style.yellow, rune.style.gray, rune.style.dim, rune.style.inverse

-- Ctrl+C double-press quit: the first press on an empty line arms quit
-- for QUIT_WINDOW seconds and the status bar says so; a second press in
-- that window quits. A timestamp rather than a timer, so nothing can
-- cancel the expiry and leave quit armed; the bar's periodic redraw
-- clears the warning.
local QUIT_WINDOW = 2
local quit_at

local function quit_armed()
    return quit_at ~= nil and rune.time() - quit_at < QUIT_WINDOW
end

-- Defaults are below; the quit state is shared with the bar.
local function on_ctrl_c()
    if rune.input.get() ~= "" then
        rune.input.set("")
        return
    end
    if quit_armed() then
        rune.quit()
    else
        quit_at = rune.time()
        rune.ui.refresh_bars()
    end
end

-- Render completion bar when cycling through matches
local function render_completion_bar()
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

-- Defaults

-- Ctrl+C: with text in the input it clears the line; on an empty line the
-- first press warns and a second press within the window quits.
rune.bind("ctrl+c", on_ctrl_c)

-- The status bar. Go re-renders it every 250ms, which also clears an
-- expired quit warning.
rune.ui.bar("status", function()
    -- Check if we should show completion bar instead (only when cycling with multiple matches)
    local comp = rune.completion and rune.completion.state
    if comp and comp.original and #comp.matches > 1 then
        return render_completion_bar()
    end

    local state = rune.state

    -- Left side: connection status (or quit warning)
    local left
    if quit_armed() then
        left = yellow("Press Ctrl+C again to exit")
    elseif state.connected then
        left = green("●") .. " " .. gray(state.address)
    else
        left = gray("●") .. " " .. gray("Disconnected")
    end

    -- Right side: scroll mode indicator
    local right
    if state.search_active then
        right = yellow("SEARCH")
    elseif state.scroll_mode == "scrolled" then
        right = yellow("SCROLL") .. " " .. dim("(" .. state.scroll_lines .. " new)")
    else
        right = dim("LIVE")
    end

    return { left = left, right = right }
end)

