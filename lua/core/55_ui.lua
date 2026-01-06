-- UI: Panes, Status Bar, and Keybindings
-- Everything visual: pane management, status rendering, picker bindings

-- ============================================================
-- PANE PRIMITIVES
-- Wrappers around Go pane API
-- ============================================================

rune.pane = {}

function rune.pane.create(name)
    rune._pane.create(name)
end

function rune.pane.write(name, text)
    rune._pane.write(name, text)
end

function rune.pane.toggle(name)
    rune._pane.toggle(name)
end

function rune.pane.clear(name)
    rune._pane.clear(name)
end

function rune.pane.scroll_up(name, lines)
    rune._pane.scroll_up(name, lines or 1)
end

function rune.pane.scroll_down(name, lines)
    rune._pane.scroll_down(name, lines or 1)
end

function rune.pane.scroll_to_top(name)
    rune._pane.scroll_to_top(name)
end

function rune.pane.scroll_to_bottom(name)
    rune._pane.scroll_to_bottom(name)
end

-- ============================================================
-- PANE SCROLLING BINDINGS
-- ============================================================

rune.bind("pageup", function() rune.pane.scroll_up("main", 20) end)
rune.bind("pagedown", function() rune.pane.scroll_down("main", 20) end)
rune.bind("shift+pageup", function() rune.pane.scroll_to_top("main") end)
rune.bind("shift+pagedown", function() rune.pane.scroll_to_bottom("main") end)

-- ============================================================
-- STATUS BAR
-- Reactive status bar using rune.ui.bar() API
-- ============================================================

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

-- ============================================================
-- PICKER BINDINGS
-- ============================================================

-- History Search (Ctrl+R)
rune.bind("ctrl+r", function()
    local history = rune.history.get()

    -- Reverse history for display (newest first)
    local items = {}
    for i = #history, 1, -1 do
        table.insert(items, history[i])
    end

    rune.ui.picker.show({
        title = "History",
        items = items,
        on_select = function(val)
            rune.input.set(val)
        end
    })
end)

-- Alias Search (Ctrl+T)
rune.bind("ctrl+t", function()
    local aliases = rune.alias.list()

    -- Format for picker
    local items = {}
    for _, a in ipairs(aliases) do
        table.insert(items, {
            text = a.match,
            desc = a.value,
            value = a.match
        })
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
        table.insert(items, {
            text = "/" .. c.name,
            desc = c.description,
            value = "/" .. c.name
        })
    end

    -- Open picker in inline mode
    -- The picker filters based on full input content
    rune.ui.picker.show({
        items = items,
        mode = "inline",
        match_description = true,
        on_select = function(val)
            -- Selection completes - the UI already set input to "/command "
        end
    })
end)
