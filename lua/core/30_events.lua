-- Default System Event Handlers
-- Users can register additional handlers or override with lower priority

-- Note: Status bar is now handled reactively by 70_status_bar.lua
-- It reads rune.state directly, so we don't need to call rune.status.set() here.

rune.hooks.register("ready", function()
    -- Status bar renders from rune.state automatically
end, { priority = 100 })

rune.hooks.register("connecting", function(addr)
    rune.print("[System] Connecting to " .. addr .. "...")
end, { priority = 100 })

rune.hooks.register("connected", function(addr)
    rune.print("[System] Connected to " .. addr)
end, { priority = 100 })

rune.hooks.register("disconnecting", function()
    rune.print("[System] Disconnecting...")
end, { priority = 100 })

rune.hooks.register("disconnected", function()
    rune.print("[System] Disconnected")
end, { priority = 100 })

rune.hooks.register("reloading", function()
    rune.print("[System] Reloading scripts...")
end, { priority = 100 })

rune.hooks.register("reloaded", function()
    rune.print("[System] Scripts reloaded")
end, { priority = 100 })

rune.hooks.register("loaded", function(path)
    rune.print("[System] Loaded: " .. path)
end, { priority = 100 })

rune.hooks.register("error", function(msg)
    rune.print("[Error] " .. msg)
end, { priority = 100 })
