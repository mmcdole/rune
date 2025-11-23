-- Example user script demonstrating the Lua API

-- Standard Aliases
rune.alias.add("tt", "take torch")
rune.alias.add("gt", "get torch")
rune.alias.add("l", "look")

-- Recursive Alias (expands into multiple commands)
rune.alias.add("path_to_fountain", "s;s;w;open gate;n")

-- Bot logic (Alias with Wait)
-- When user types 'bot', it expands.
-- Expansion hits #wait, which calls Go's timer, which calls back Lua.
rune.alias.add("bot", "kill orc; #wait 3; get coins; #wait 1; s")

-- Nested alias example
rune.alias.add("prep", "tt;l")
