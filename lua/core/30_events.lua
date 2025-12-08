-- Default System Event Handlers
-- Users can add handlers or override with lower priority

-- Note: Status bar is now handled reactively by 70_status_bar.lua
-- It reads rune.state directly, so we don't need to call rune.status.set() here.

rune.hooks.on("ready", function()
    -- Status bar renders from rune.state automatically
end, { priority = 100 })

rune.hooks.on("connecting", function(addr)
    rune.echo("[System] Connecting to " .. addr .. "...")
end, { priority = 100 })

rune.hooks.on("connected", function(addr)
    rune.echo("[System] Connected to " .. addr)
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
