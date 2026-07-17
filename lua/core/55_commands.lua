-- Slash Command System
-- Built on rune.registry (15_registry.lua), so commands get the same
-- upsert-by-name, source attribution, and failure quarantine as every
-- other callback registry. A command that keeps throwing is disabled
-- individually - it can never take the core input hook down with it.

-- Styling shorthands (see 05_style.lua)
local green, red, yellow, cyan, dim =
    rune.style.green, rune.style.red, rune.style.yellow,
    rune.style.cyan, rune.style.gray

local by_cmd = {} -- command name -> data

local registry = rune.registry.new{
    kind = "command",
    on_add = function(data)
        -- Upsert by command name: re-adding replaces the old handler
        local old = by_cmd[data.command]
        if old and old ~= data then
            old._handle:remove()
        end
        by_cmd[data.command] = data
    end,
    on_remove = function(data)
        if by_cmd[data.command] == data then
            by_cmd[data.command] = nil
        end
    end,
}

rune.command = {}

-- Add a slash command. opts: group (see 15_registry.lua).
-- Returns a handle with :enable/:disable/:remove.
function rune.command.add(name, handler, description, opts)
    return registry:add({
        command = name,
        handler = handler,
        description = description or "",
        source = rune.caller_source(1),
    }, { name = name, group = opts and opts.group })
end

-- Remove a slash command by name. Returns true if one existed.
function rune.command.remove(name)
    return registry:remove(name)
end

-- Get a slash command handler (unwrapped, no quarantine)
function rune.command.get(name)
    local data = by_cmd[name]
    return data and data.handler or nil
end

-- INTERNAL: run a command protected (called by the core input hook).
-- Returns true if the name was a known command, even when it is
-- disabled or its handler failed - the input is consumed either way.
function rune.command.dispatch(name, args)
    local data = by_cmd[name]
    if not data then
        return false
    end
    if not registry:active(data) then
        rune.echo(red("[Error]") .. " /" .. name .. " is disabled" ..
            " (re-enable with rune.command.enable)")
        return true
    end
    local label = 'Command "/' .. name .. '"' ..
        (data.source and (" @" .. data.source) or "")
    rune.guarded_call(label, data, data.handler, args)
    return true
end

-- Management by name
function rune.command.enable(name)
    return registry:enable(name)
end

function rune.command.disable(name)
    return registry:disable(name)
end

-- List all commands for the picker and /help.
-- Returns array of {name, description, enabled, group, source}.
function rune.command.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
        table.insert(result, {
            name = data.command,
            description = data.description,
            enabled = data.enabled,
            group = data.group,
            source = data.source,
        })
    end
    -- Sort alphabetically
    table.sort(result, function(a, b) return a.name < b.name end)
    return result
end

-- /connect - three forms, resolved in order:
--   no args              -> picker over saved worlds (65_worlds.lua)
--   <world name>         -> saved world's address
--   <host> <port> [tls|tls+insecure], or a bare address ("host:port",
--                           optionally with a scheme) -> direct
-- rune.world is checked defensively: it loads after this file, and
-- may be missing in degraded mode.
local USAGE_CONNECT = "[Usage] /connect <world> | /connect <host> <port> [tls|tls+insecure]"

rune.command.add("connect", function(args)
    if args == "" then
        local worlds = rune.world and rune.world.list() or {}
        if #worlds == 0 then
            rune.echo(USAGE_CONNECT)
            return
        end
        local items = {}
        for _, w in ipairs(worlds) do
            table.insert(items, { text = w.name, desc = w.address, value = w.name })
        end
        rune.ui.picker.show{
            title = "Connect",
            items = items,
            match_description = true,
            on_select = function(name)
                local entry = rune.world and rune.world.get(name)
                if entry then
                    rune.connect(entry.address)
                end
            end,
        }
        return
    end

    local word = args:match("^(%S+)$")
    if word and rune.world then
        local entry = rune.world.get(word)
        if entry then
            rune.connect(entry.address)
            return
        end
    end

    local host, port, scheme = args:match("^(%S+)%s+(%d+)%s*(%S*)$")
    if host and port then
        if scheme == "" then
            rune.connect(host .. ":" .. port)
        elseif scheme == "tls" or scheme == "tls+insecure" then
            rune.connect(scheme .. "://" .. host .. ":" .. port)
        else
            rune.echo(USAGE_CONNECT)
        end
        return
    end
    if word and word:find(":") then
        rune.connect(word)
    else
        rune.echo(USAGE_CONNECT)
    end
end, "Connect to a world or address")

-- /disconnect - Disconnect from server
rune.command.add("disconnect", function(args)
    rune.disconnect()
end, "Disconnect from server")

-- /reconnect - Reconnect to last server. The address is stored in
-- rune.store (on the "connected" event), so it survives /reload and
-- client restarts.
rune.command.add("reconnect", function(args)
    local addr = rune.store.get("last_address")
    if addr then
        rune.connect(addr)
    else
        rune.echo("[Error] No previous connection")
    end
end, "Reconnect to last server")

-- /load <path> - Load a Lua script
rune.command.add("load", function(args)
    if args == "" then
        rune.echo("[Usage] /load <path>")
        return
    end
    local ok, err = rune.load(args)
    if ok then
        rune.echo("[Loaded] " .. args)
    else
        rune.echo("[Error] " .. tostring(err))
    end
end, "Load a Lua script file")

-- /reload - Clear state and reload init.lua
rune.command.add("reload", function(args)
    rune.reload()
end, "Reload all scripts")

-- /lua <code> - Execute Lua code directly
rune.command.add("lua", function(args)
    if args == "" then
        rune.echo("[Usage] /lua <code>")
        return
    end

    local fn, err = loadstring(args)
    if fn then
        local ok, result = pcall(fn)
        if ok then
            if result ~= nil then
                rune.echo(tostring(result))
            end
        else
            rune.echo("[Error] " .. tostring(result))
        end
    else
        rune.echo("[Error] " .. tostring(err))
    end
end, "Execute Lua code")

-- /aliases - List all aliases
rune.command.add("aliases", function(args)
    local aliases = rune.alias.list()
    rune.echo(green("[Aliases]") .. dim(" (" .. #aliases .. " total)"))
    if #aliases == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, a in ipairs(aliases) do
        local status = a.enabled and green("[on] ") or red("[off]")
        local group_str = a.group and ("  " .. cyan("<" .. a.group .. ">")) or ""
        local flags = {}
        if a.once then flags[#flags + 1] = "once" end
        local name_str = a.name and (" " .. dim("name:") .. a.name) or ""
        local flags_str = #flags > 0 and ("  " .. dim("(" .. table.concat(flags, ", ") .. ")")) or ""
        local src_str = a.source and ("  " .. dim("@" .. a.source)) or ""
        rune.echo(string.format("  %s %-8s %s %s %s %s%s%s%s",
            status, a.mode, yellow('"' .. a.match .. '"'), dim("->"), a.value, name_str, group_str, flags_str, src_str))
    end
end, "List all aliases")

-- /triggers - List all triggers
rune.command.add("triggers", function(args)
    local triggers = rune.trigger.list()
    rune.echo(green("[Triggers]") .. dim(" (" .. #triggers .. " total)"))
    if #triggers == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, t in ipairs(triggers) do
        local status = t.enabled and green("[on] ") or red("[off]")
        local group_str = t.group and ("  " .. cyan("<" .. t.group .. ">")) or ""
        local flags = {}
        if t.gag then flags[#flags + 1] = "gag" end
        if t.once then flags[#flags + 1] = "once" end
        if t.raw then flags[#flags + 1] = "raw" end
        if t.span then flags[#flags + 1] = "span" end
        local name_str = t.name and (" " .. dim("name:") .. t.name) or ""
        local flags_str = #flags > 0 and ("  " .. dim("(" .. table.concat(flags, ", ") .. ")")) or ""
        local src_str = t.source and ("  " .. dim("@" .. t.source)) or ""
        rune.echo(string.format("  %s %-8s %s %s %s%s%s%s%s",
            status, t.mode, yellow('"' .. t.match .. '"'), dim("->"), t.value, name_str, group_str, flags_str, src_str))
    end
end, "List all triggers")

-- /test <line> - Simulate server output (test triggers)
rune.command.add("test", function(args)
    if args == "" then
        rune.echo("[Usage] /test <line>")
        return
    end

    rune.echo("[Test Input] " .. args)

    local modified, show = rune.trigger.process(rune.line.new(args))
    if show and modified ~= "" then
        rune.echo("[Test Output] " .. modified)
    else
        rune.echo("[Test Output] (gagged)")
    end
end, "Test triggers with simulated line")

-- /timers - List all timers
rune.command.add("timers", function(args)
    local timers = rune.timer.list()
    rune.echo(green("[Timers]") .. dim(" (" .. #timers .. " total)"))
    if #timers == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, t in ipairs(timers) do
        local status = t.enabled and green("[on] ") or red("[off]")
        local group_str = t.group and ("  " .. cyan("<" .. t.group .. ">")) or ""
        local name_str = t.name and (" " .. dim("name:") .. t.name) or ""
        local src_str = t.source and ("  " .. dim("@" .. t.source)) or ""
        local timing = string.format("%s %.1fs", t.mode, t.seconds)
        rune.echo(string.format("  %s %-12s %s %s%s%s%s",
            status, timing, dim("->"), t.value, group_str, name_str, src_str))
    end
end, "List all timers")

-- /hooks - List all hooks
rune.command.add("hooks", function(args)
    local hooks = rune.hooks.list()
    rune.echo(green("[Hooks]") .. dim(" (" .. #hooks .. " total)"))
    if #hooks == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, h in ipairs(hooks) do
        local status = h.enabled and green("[on] ") or red("[off]")
        local group_str = h.group and ("  " .. cyan("<" .. h.group .. ">")) or ""
        local name_str = h.name or "(anonymous)"
        local pri_str = dim("pri:") .. tostring(h.priority)
        local src_str = h.source and ("  " .. dim("@" .. h.source)) or ""
        rune.echo(string.format("  %s %-12s %s %s %s%s%s",
            status, h.event, pri_str, dim("->"), name_str, group_str, src_str))
    end
end, "List all hooks")

-- /binds - List all key bindings
rune.command.add("binds", function(args)
    local binds = rune.binds.list()
    rune.echo(green("[Binds]") .. dim(" (" .. #binds .. " total)"))
    if #binds == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, b in ipairs(binds) do
        local status = b.enabled and green("[on] ") or red("[off]")
        local group_str = b.group and ("  " .. cyan("<" .. b.group .. ">")) or ""
        local name_str = b.name and ("  " .. dim("name:") .. b.name) or ""
        local src_str = b.source and ("  " .. dim("@" .. b.source)) or ""
        rune.echo(string.format("  %s %-16s%s%s%s",
            status, yellow(b.key), group_str, name_str, src_str))
    end
end, "List all key bindings")

-- /bars - List all bar renderers
rune.command.add("bars", function(args)
    local bars = rune.bars.list()
    rune.echo(green("[Bars]") .. dim(" (" .. #bars .. " total)"))
    if #bars == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, b in ipairs(bars) do
        local status = b.enabled and green("[on] ") or red("[off]")
        local group_str = b.group and ("  " .. cyan("<" .. b.group .. ">")) or ""
        local src_str = b.source and ("  " .. dim("@" .. b.source)) or ""
        rune.echo(string.format("  %s %-16s%s%s",
            status, yellow(b.bar), group_str, src_str))
    end
end, "List all bar renderers")

-- /groups - List all groups and their enabled state
rune.command.add("groups", function(args)
    local groups = rune.group.list()
    rune.echo(green("[Groups]") .. dim(" (" .. #groups .. " total)"))
    if #groups == 0 then
        rune.echo("  " .. dim("(none)"))
        return
    end
    for _, g in ipairs(groups) do
        local status = g.enabled and green("[on] ") or red("[off]")
        rune.echo("  " .. status .. " " .. g.name)
    end
end, "List all groups")

-- /group <name> on|off - Master switch for a group, so a pack of
-- triggers/aliases can be toggled mid-game without typing Lua.
rune.command.add("group", function(args)
    local name, action = args:match("^(%S+)%s+(%S+)$")
    if not name or (action ~= "on" and action ~= "off") then
        rune.echo("[Usage] /group <name> on|off")
        return
    end
    local known = false
    for _, g in ipairs(rune.group.list()) do
        if g.name == name then known = true break end
    end
    if action == "on" then
        rune.group.enable(name)
        rune.echo(green("[Group]") .. " " .. name .. " enabled" ..
            (known and "" or dim("  (no items in this group yet)")))
    else
        rune.group.disable(name)
        rune.echo(yellow("[Group]") .. " " .. name .. " disabled" ..
            (known and "" or dim("  (no items in this group yet)")))
    end
end, "Enable/disable a group (/group <name> on|off)")

-- /raw <text> - Send without alias expansion
rune.command.add("raw", function(args)
    if args == "" then
        rune.echo("[Usage] /raw <text>")
        return
    end
    rune.send_raw(args)
end, "Send text without alias expansion")

-- /echo <text> - Print to the local screen (never sent to the server).
-- Handy for testing and for use in alias/bind command strings.
rune.command.add("echo", function(args)
    rune.echo(args)
end, "Print text locally")

-- /version - Client version
rune.command.add("version", function(args)
    rune.echo("Rune " .. rune.version)
end, "Show client version")

-- /quit - Exit the client
rune.command.add("quit", function(args)
    rune.echo("[System] Goodbye!")
    rune.quit()
end, "Exit the client")

-- /help - Show available commands, generated from the registry so
-- user-added commands appear automatically and descriptions cannot
-- drift from what the picker shows.
rune.command.add("help", function(args)
    local cmds = rune.command.list()
    rune.echo(green("[Commands]") .. dim(" (" .. #cmds .. " total)"))
    for _, c in ipairs(cmds) do
        local status = c.enabled and "" or (" " .. red("[off]"))
        rune.echo(string.format("  %-12s %s%s",
            "/" .. c.name, c.description, status))
    end
end, "Show available commands")
