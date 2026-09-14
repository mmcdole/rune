-- Register output handler
rune.hooks.on("output", function(line)
    local modified, show = rune.trigger._process_output(line)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })

-- Prompt triggers opt into partial lines, which may repeat as they grow.
rune.hooks.on("prompt", function(line, confirmed)
    local modified, show = rune.trigger._process_prompt(line, confirmed)
    if not show then
        return false
    end
    return modified
end, { priority = 100 })
