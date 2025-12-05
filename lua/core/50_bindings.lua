-- Default Key Bindings

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
