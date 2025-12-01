-- Default Key Bindings
-- Uses the reactive rune.ui.picker and rune.history APIs

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
    local aliases = rune.alias.all()

    -- Format for picker
    local items = {}
    for _, a in ipairs(aliases) do
        table.insert(items, {
            text = a.name,
            desc = a.value,
            value = a.name
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
