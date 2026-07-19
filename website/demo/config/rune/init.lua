-- Demo profile for the website VHS gifs. Seeds the same worlds, aliases,
-- and history the landing page's simulated terminals show, so the real
-- client demos match the site's story world.

-- Worlds (the /connect picker).
rune.world.add("viking", "vikingmud.org:2001")
rune.world.add("arctic", "mud.arctic.org:2700")
rune.world.add("discworld", "discworld.starturtle.net:4242")

-- Aliases (the Ctrl+T picker).
-- Exact aliases read best in the picker (regex entries display their
-- pattern); bs/tt exist for the Ctrl+T frame and are never invoked.
rune.alias.exact("ws", "wield sword;wear shield")
rune.alias.exact("gac", "get all from corpse")
rune.alias.exact("bs", "cast blindstrike at %1")
rune.alias.exact("tt", "tell tundra %*")

-- Quake-style chat console (the hero's closing beat): channel lines
-- mirror into a dockable pane; backtick toggles it. This is the whole
-- implementation - the landing page brags about exactly these lines.
rune.ui.layout({
    top = { { name = "chat", height = 6 } },
    bottom = { "input", "status" },
})
rune.trigger.regex("^\\[(Chat|Trade|Tell)\\]", function(_, ctx)
    rune.pane.write("chat", ctx.line:raw())
end)
rune.bind("`", function() rune.pane.toggle("chat") end)

-- History (the Ctrl+R picker), oldest first.
rune.history.add("/world add arctic mud.arctic.org 2700")
rune.history.add("tell dios when is the restart?")
rune.history.add("get all from corpse")
rune.history.add("tell soblak ready when you are")
rune.history.add("kill polar bear")
