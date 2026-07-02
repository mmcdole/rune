-- Default System Event Handlers
-- Users can add handlers or override with lower priority.
-- The status bar (55_ui.lua) renders reactively from rune.state, so
-- these handlers only produce the scrollback notices.

rune.hooks.on("ready", function()
    -- Boot complete; nothing to do by default
end, { priority = 100 })

rune.hooks.on("connecting", function(addr)
    rune.echo("[System] Connecting to " .. addr .. "...")
end, { priority = 100 })

rune.hooks.on("connected", function(addr)
    rune.echo("[System] Connected to " .. addr)
    -- Remember across /reload so /reconnect keeps working
    rune.persist.set("last_address", addr)
end, { priority = 100 })

rune.hooks.on("disconnecting", function()
    rune.echo("[System] Disconnecting...")
end, { priority = 100 })

rune.hooks.on("disconnected", function()
    rune.echo("[System] Disconnected")
end, { priority = 100 })

rune.hooks.on("reloading", function()
    rune.echo("[System] Reloading scripts...")
end, { priority = 100 })

rune.hooks.on("reloaded", function()
    rune.echo("[System] Scripts reloaded")
end, { priority = 100 })

rune.hooks.on("loaded", function(path)
    rune.echo("[System] Loaded: " .. path)
end, { priority = 100 })

rune.hooks.on("error", function(msg)
    rune.echo("[Error] " .. msg)
end, { priority = 100 })
