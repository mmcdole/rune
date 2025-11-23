-- Core Configuration
-- The rune table is created by Go, we add config here

rune.config = {
    delimiter = ";",
    wait_cmd = "#wait"
}

-- System Event Hooks
-- These are called by Go to notify Lua of system events.
-- Users can override these in their init.lua for custom styling.

function on_sys_connecting(addr)
    rune.print("[System] Connecting to " .. addr .. "...")
end

function on_sys_connected(addr)
    rune.print("[System] Connected to " .. addr)
end

function on_sys_disconnecting()
    rune.print("[System] Disconnecting...")
end

function on_sys_disconnected()
    rune.print("[System] Disconnected")
end

function on_sys_reloading()
    rune.print("[System] Reloading scripts...")
end

function on_sys_reloaded()
    rune.print("[System] Scripts reloaded")
end

function on_sys_loaded(path)
    rune.print("[System] Loaded: " .. path)
end

function on_sys_error(msg)
    rune.print("[Error] " .. msg)
end
