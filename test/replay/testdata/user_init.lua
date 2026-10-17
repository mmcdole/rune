-- A representative combat script used by the replay benchmarks: literal and
-- regex triggers, a gag, a prompt trigger, GMCP handlers, a status bar, and a
-- chat pane. It never sends anything, so a replay is side-effect free.

local stats = { hits = 0, misses = 0, damage_in = 0, hp = 0, sp = 0, enemy = "" }

for _, word in ipairs({ "scratch", "hit", "smash", "massacre", "tickle" }) do
    rune.trigger.contains("You " .. word .. " ", function() stats.hits = stats.hits + 1 end,
        { group = "combat" })
end
rune.trigger.contains("You miss ", function() stats.misses = stats.misses + 1 end, { group = "combat" })
rune.trigger.starts("You quaff", function() end, { group = "combat" })
rune.trigger.exact("You feel a surge of power as your weapon glows!", function()
    rune.echo(rune.style.yellow("*** WEAPON PROC ***"))
end)

rune.trigger.regex("^(The .+|Vulcan the smith|Grimbold the berserker|A cave bear) (\\w+(?: \\w+)*) you in the (.+)\\.$",
    function(m) stats.damage_in = stats.damage_in + 1 end, { group = "combat" })
rune.trigger.regex("^(.+) is in (excellent|good|bad|very bad|awful) shape\\.$",
    function(m) stats.enemy = m[1] end, { group = "combat" })
rune.trigger.regex("^Thorvald (\\w+) (.+)\\.$", function() end, { gag = true })
rune.trigger.regex("^\\[(\\w+)\\] (\\w+): (.*)$", function(m)
    rune.pane.write("chat", m[2] .. ": " .. m[3])
end)
for i = 1, 12 do
    rune.trigger.regex("^Unused pattern number " .. i .. " (\\d+)$", function() end)
end

rune.trigger.regex("HP:(\\d+)/(\\d+) SP:(\\d+)/(\\d+)", function(m)
    stats.hp = tonumber(m[1])
end, { on = "prompt" })

rune.gmcp.on("Char.Vitals", function(data)
    stats.hp, stats.sp = data.hp, data.sp
end)
rune.gmcp.on("Room.Info", function(data) stats.room = data.name end)

rune.ui.bar("combat", function(width)
    return {
        left = string.format("HP %d SP %d", stats.hp or 0, stats.sp or 0),
        center = stats.enemy,
        right = string.format("hits %d / miss %d", stats.hits, stats.misses),
    }
end)

rune.alias.exact("k", "kill troll")
