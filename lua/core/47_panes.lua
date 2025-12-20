-- Pane scrolling (Option B: Go primitives + Lua logic)
-- Users can override any of these behaviors

-- ============================================================
-- Main Buffer Scrolling
-- ============================================================
rune.bind("pageup", function() rune.pane.scroll_up("main", 20) end)
rune.bind("pagedown", function() rune.pane.scroll_down("main", 20) end)
rune.bind("shift+pageup", function() rune.pane.scroll_to_top("main") end)
rune.bind("shift+pagedown", function() rune.pane.scroll_to_bottom("main") end)
