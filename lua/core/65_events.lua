-- Default System Event Handlers
-- Users can add handlers or override with lower priority.
-- The status bar (75_ui.lua) renders reactively from rune.state, so
-- these handlers only produce the scrollback notices.

-- Local echo styling. This is the only place the "> " prefix and its
-- color exist; return false from an earlier-priority handler to hide
-- an echo, or a string to restyle it.
rune.hooks.on("echo", function(text)
    return rune.style.green("> " .. text)
end, { priority = 100 })

rune.hooks.on("connecting", function(addr)
    rune.echo("[System] Connecting to " .. addr .. "...")
end, { priority = 100 })

rune.hooks.on("connected", function(addr)
    rune.echo("[System] Connected to " .. addr)
    -- Remember durably so /reconnect works across /reload AND restarts
    rune.store.set("last_address", addr)
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
