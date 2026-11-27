-- UI
-- The screen: layout, regions, panes, pickers, scrollback search, and the
-- clipboard, plus Rune's default layout and scroll keys.
--
-- Go owns the terminal UI (rune._ui, rune._pane) and keeps output and input
-- usable before the core loads; this file defines the public API and the
-- default layout.
--
-- API:
--   rune.ui.layout(root)                      -- install a layout tree
--   rune.ui.regions.show/hide/toggle/is_hidden(id)
--   rune.ui.refresh_bars()                    -- re-render bars after this event
--   rune.ui.picker.show(opts)                 -- picker overlay
--   rune.ui.search(opts?)                     -- scrollback search overlay
--   rune.pane.create/write/replace/clear(name, ...)
--   rune.pane.show/hide/toggle/is_hidden(name)
--   rune.pane.scroll_up/scroll_down(name, lines), scroll_to_top/bottom(name)
--   rune.clipboard.set(text)                  -- OSC 52
--
-- Notes:
--   rune.ui.bar is defined in bars.lua, which owns the bar registry.

-- Layout and regions

rune.ui = {}

-- Install a layout from a root node. Returns true once the tree replaces the
-- active layout; an invalid tree raises. The retired top/bottom table form is
-- rejected with a notice and returns false instead of raising, because raising
-- would abort the rest of the script that carried it.
function rune.ui.layout(config)
    if type(config) == "table" and config.type == nil
        and (config.top ~= nil or config.bottom ~= nil or config.version ~= nil
            or next(config) == nil) then
        local source = rune.caller_source(1)
        rune.echo(rune.style.red("[Error]") .. " rune.ui.layout: top/bottom layout tables are no longer supported"
            .. (source and (" (" .. source .. ")") or ""))
        rune.echo("  Pass a root node. See https://runemud.com/interface/layout/#migrating-from-topbottom-tables")
        return false
    end
    rune._ui.layout(config)
    return true
end

-- A row or column with an id is a region. Region visibility hides or shows its
-- subtree without changing the visibility or enabled state of contained resources.
rune.ui.regions = {}

function rune.ui.regions.show(id)
    return rune._ui.region_show(id)
end

function rune.ui.regions.hide(id)
    return rune._ui.region_hide(id)
end

function rune.ui.regions.toggle(id)
    return rune._ui.region_toggle(id)
end

function rune.ui.regions.is_hidden(id)
    return rune._ui.region_is_hidden(id)
end

-- Render bars when the current event finishes.
function rune.ui.refresh_bars()
    rune._ui.bars_changed()
end

-- Pickers

rune.ui.picker = {}

-- Picker id -> on_select. Pending callbacks die with the VM on /reload.
-- Go allocates ids that never repeat, so a result for a picker shown
-- before the reload is dropped.
local pending_pickers = {}

-- Show a picker overlay.
-- opts = {
--   title = "History",              -- optional (modal mode only)
--   items = {"a", "b"} or {{text=..., value=..., desc=...}},
--   on_select = function(value) end,
--   mode = "inline",                -- optional: "inline" or "modal" (default)
--   match_description = true,       -- optional: fuzzy-match descriptions too
-- }
function rune.ui.picker.show(opts)
    if type(opts) ~= "table" then
        error("rune.ui.picker.show: opts must be a table", 2)
    end
    if type(opts.on_select) ~= "function" then
        error("rune.ui.picker.show: on_select must be a function", 2)
    end
    local id = rune._ui.picker_show(opts)
    pending_pickers[id] = opts.on_select
end

-- INTERNAL: called by Go when a picker closes. on_select runs only for an
-- accepted selection; a throwing callback propagates to the engine's
-- error report.
function rune.ui.picker._settle(id, accepted, value)
    local on_select = pending_pickers[id]
    pending_pickers[id] = nil
    if on_select and accepted then
        on_select(value)
    end
end

-- Search

-- Open the scrollback-search overlay (also bound to Ctrl+F and /find).
-- opts = { query = "thief" }  -- optional; omitted or empty reopens
--                             -- with the previous search's query
function rune.ui.search(opts)
    rune._ui.search_show(opts or {})
end

-- Panes

rune.pane = {}

function rune.pane.create(name)
    rune._pane.create(name)
end

function rune.pane.write(name, text)
    rune._pane.write(name, text)
end

-- Replace the whole buffer in one UI update. Use this for panes redrawn as a
-- block (status forms, group lists) instead of clear followed by writes, which
-- can show an empty frame in between. Creates the pane like write does; a
-- scrolled pane returns to live.
function rune.pane.replace(name, text)
    rune._pane.replace(name, text)
end

-- Visibility is placement state on the active layout tree: show, hide, and
-- toggle flip the pane's placement and return whether the layout places the
-- pane at all. A new rune.ui.layout() or /reload restores declared state.
function rune.pane.toggle(name)
    return rune._pane.toggle(name)
end

function rune.pane.show(name)
    return rune._pane.show(name)
end

function rune.pane.hide(name)
    return rune._pane.hide(name)
end

-- Reports the pane's local hidden state (not ancestors), or nil when
-- the layout does not place the pane.
function rune.pane.is_hidden(name)
    return rune._pane.is_hidden(name)
end

function rune.pane.clear(name)
    rune._pane.clear(name)
end

function rune.pane.scroll_up(name, lines)
    rune._pane.scroll_up(name, lines)
end

function rune.pane.scroll_down(name, lines)
    rune._pane.scroll_down(name, lines)
end

function rune.pane.scroll_to_top(name)
    rune._pane.scroll_to_top(name)
end

function rune.pane.scroll_to_bottom(name)
    rune._pane.scroll_to_bottom(name)
end

-- Clipboard

rune.clipboard = {}

-- Copy text to the system clipboard. Uses OSC 52, so it works over
-- SSH; the terminal emulator has to support it.
function rune.clipboard.set(text)
    rune._ui.set_clipboard(text)
end

-- Defaults

-- The default layout: output, the input line, and the status bar.
rune.ui.layout({
    type = "column",
    children = {
        { type = "pane", name = "output", border = "none" },
        { type = "input" },
        { type = "bar", name = "status" },
    },
})

rune.bind("pgup", function() rune.pane.scroll_up("output", 20) end)
rune.bind("pgdown", function() rune.pane.scroll_down("output", 20) end)
-- Bare Home/End are deliberately unbound: they fall through to the
-- input widget as cursor-to-start/end, matching the composer's keymap.
rune.bind("ctrl+home", function() rune.pane.scroll_to_top("output") end)
rune.bind("ctrl+end", function() rune.pane.scroll_to_bottom("output") end)

-- Scrollback Search (Ctrl+F)
-- Reopening preserves the last query; typing replaces it. The overlay
-- itself owns stepping (Up/Down), commit (Enter), and cancel (Esc).
rune.bind("ctrl+f", function()
    rune.ui.search()
end)

-- /find [pattern] - Search the scrollback. With no pattern the overlay
-- reopens with the previous query (find-next lives in the overlay).
rune.command.add("find", function(args)
    rune.ui.search({ query = args })
end, "Search scrollback")
