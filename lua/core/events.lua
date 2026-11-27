-- Notices and local echo styling
-- The scrollback messages Rune prints for system events, and the "> "
-- styling of echoed input. Each is a named hook, so a script can hide
-- one (rune.hooks.disable("notice-reloaded")) or replace it by
-- registering its own handler under the same name. Notification hooks
-- ignore return values, so disabling by name is how to silence one.
-- The status bar (ui.lua) renders reactively from rune.state.

-- Local echo styling. Re-register "echo-style" to restyle echoed input
-- (Go keeps a plain fallback for when the Lua core is unavailable).
rune.hooks.on("echo", function(text)
    return rune.style.green("> " .. text)
end, { name = "echo-style", priority = 100 })

-- First-run welcome: shown at startup only while no init.lua exists, so
-- new users learn where config lives and how to connect; it disappears
-- the moment they create one. Plain startup code, not a hook: disabling
-- a hook would need the very init.lua whose absence shows it.
if rune.config_dir then
    local init_path = rune.config_dir .. "/init.lua"
    local f = io.open(init_path, "r")
    if f then
        f:close()
    else
        local gray, cyan = rune.style.gray, rune.style.cyan
        rune.echo("")
        rune.echo(gray("Welcome! To get started:"))
        rune.echo(gray("  connect:   ") .. cyan("/connect <host> <port>") ..
            gray("  (or ") .. cyan("tls") .. gray(" as a third argument)"))
        rune.echo(gray("  bookmark:  ") .. cyan("/world add <name> <host> <port>") ..
            gray("  then ") .. cyan("/connect <name>"))
        rune.echo(gray("  scripting: ") .. cyan(init_path) .. gray(" auto-loads at startup"))
        rune.echo(gray("  docs:      ") .. cyan("https://github.com/mmcdole/rune"))
        rune.echo("")
    end
end

-- [System] notices, one named hook per event: notice-<event>.
local function notice(event, render)
    rune.hooks.on(event, function(...)
        rune.echo(render(...))
    end, { name = "notice-" .. event, priority = 100 })
end

notice("connecting", function(addr) return "[System] Connecting to " .. addr .. "..." end)
notice("connected", function(addr) return "[System] Connected to " .. addr end)
notice("disconnecting", function() return "[System] Disconnecting..." end)
notice("disconnected", function() return "[System] Disconnected" end)
notice("reloading", function() return "[System] Reloading scripts..." end)
notice("reloaded", function() return "[System] Scripts reloaded" end)
notice("loaded", function(path) return "[System] Loaded: " .. path end)
notice("error", function(msg) return rune.style.red("[Error]") .. " " .. msg end)
