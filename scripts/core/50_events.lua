-- Default System Event Handlers
-- Users can register additional handlers or override with lower priority

-- Track current connection for status bar
local _current_server = nil

rune.hooks.register("ready", function()
    rune.print("Rune MUD Client")
    rune.print("Type /help for commands")
    rune.status.set("\027[90m● Disconnected\027[0m")
end, { priority = 100 })

rune.hooks.register("connecting", function(addr)
    rune.print("[System] Connecting to " .. addr .. "...")
    rune.status.set("\027[33m● Connecting to " .. addr .. "...\027[0m")
end, { priority = 100 })

rune.hooks.register("connected", function(addr)
    _current_server = addr
    rune.print("[System] Connected to " .. addr)
    -- Green dot, subdued gray text for address
    rune.status.set("\027[32m●\027[0m \027[90m" .. addr .. "\027[0m")
end, { priority = 100 })

rune.hooks.register("disconnecting", function()
    rune.print("[System] Disconnecting...")
    rune.status.set("\027[33m● Disconnecting...\027[0m")
end, { priority = 100 })

rune.hooks.register("disconnected", function()
    _current_server = nil
    rune.print("[System] Disconnected")
    rune.status.set("\027[90m● Disconnected\027[0m")
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
