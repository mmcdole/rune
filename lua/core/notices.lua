-- Notices
-- What Rune says to you: the startup banner, the first-run welcome, and
-- the [System]/[Error] lines for client events.
--
-- Go fires the events; this file owns the messages. It loads last, so the
-- banner only appears once the whole core has loaded.
--
-- Notes:
--   Each notice is a named hook, notice-<event>: disable one to silence it
--   (rune.hooks.disable("notice-reloaded")), or register your own handler
--   under the name to restyle it. Notification hooks ignore return values.

local red, gray, cyan = rune.style.red, rune.style.gray, rune.style.cyan

-- Startup

rune.echo("Rune MUD Client " .. rune.version .. " (lua: " .. rune.engine .. ")")
rune.echo("Type /help for commands")

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

-- Defaults

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
notice("error", function(msg) return red("[Error]") .. " " .. msg end)
