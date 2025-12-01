-- Slash Command Picker (Inline Mode)
-- Opens a picker that filters as you type after "/"

-- Bind "/" to open the inline picker when input is empty
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
        match_description = true,  -- Include description in fuzzy matching
        on_select = function(val)
            -- Selection completes - the UI already set input to "/command "
            -- No action needed here
        end
    })
end)
