-- Example user script demonstrating the Lua API

-- Standard Aliases
rune.alias.add("tt", "take torch")
rune.alias.add("gt", "get torch")
rune.alias.add("l", "look")

-- Recursive Alias (expands into multiple commands)
rune.alias.add("path_to_fountain", "s;s;w;open gate;n")

-- Bot logic (Function alias with delays)
-- Use rune.delay() for asynchronous timing
rune.alias.add("bot", function()
    rune.send("kill orc")
    rune.delay(3, "get coins")
    rune.delay(4, "s")
end)

-- Nested alias example
rune.alias.add("prep", "tt;l")
