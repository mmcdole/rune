-- Slash Command System

-- Styling shorthands (see 01_style.lua)
local green, red, yellow, cyan, dim =
    rune.style.green, rune.style.red, rune.style.yellow,
    rune.style.cyan, rune.style.gray

rune.command = {}
local commands = {}  -- Private storage: name -> {handler, description}

-- Add a slash command
-- rune.command.add(name, handler) or rune.command.add(name, handler, description)
function rune.command.add(name, handler, description)
    commands[name] = {
        handler = handler,
        description = description or ""
    }
end

-- Get a slash command handler
function rune.command.get(name)
    local cmd = commands[name]
    return cmd and cmd.handler or nil
end

-- List all commands for the picker (returns array of {name, description})
function rune.command.list()
    local result = {}
    for name, cmd in pairs(commands) do
        table.insert(result, {name = name, description = cmd.description})
    end
    -- Sort alphabetically
    table.sort(result, function(a, b) return a.name < b.name end)
    return result
end

-- /connect <host> <port> - Connect to server
rune.command.add("connect", function(args)
    local host, port = args:match("^(%S+)%s+(%d+)$")
    if host and port then
        rune.connect(host .. ":" .. port)
    else
        rune.echo("[Usage] /connect <host> <port>")
    end
end, "Connect to a MUD server")

-- /disconnect - Disconnect from server
rune.command.add("disconnect", function(args)
    rune.disconnect()
end, "Disconnect from server")

-- /reconnect - Reconnect to last server. The address is stored in
-- rune.persist (on the "connected" event), so it survives /reload.
rune.command.add("reconnect", function(args)
    local addr = rune.persist.get("last_address")
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
        local flags_str = #flags > 0 and ("  " .. dim("(" .. table.concat(flags, ", ") .. ")")) or ""
        local src_str = a.source and ("  " .. dim("@" .. a.source)) or ""
        rune.echo(string.format("  %s %-8s %s %s %s%s%s%s",
            status, a.mode, yellow('"' .. a.match .. '"'), dim("->"), a.value, group_str, flags_str, src_str))
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
        local flags_str = #flags > 0 and ("  " .. dim("(" .. table.concat(flags, ", ") .. ")")) or ""
        local src_str = t.source and ("  " .. dim("@" .. t.source)) or ""
        rune.echo(string.format("  %s %-8s %s %s %s%s%s%s",
            status, t.mode, yellow('"' .. t.match .. '"'), dim("->"), t.value, group_str, flags_str, src_str))
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

-- /raw <text> - Send without alias expansion
rune.command.add("raw", function(args)
    if args == "" then
        rune.echo("[Usage] /raw <text>")
        return
    end
    rune.send_raw(args)
end, "Send text without alias expansion")

-- /quit - Exit the client
rune.command.add("quit", function(args)
    rune.echo("[System] Goodbye!")
    rune.quit()
end, "Exit the client")

-- /help - Show available commands
rune.command.add("help", function(args)
    rune.echo(green("[Connection]"))
    rune.echo("  /connect <host> <port>  - Connect to server")
    rune.echo("  /disconnect             - Disconnect")
    rune.echo("  /reconnect              - Reconnect to last server")
    rune.echo("")
    rune.echo(green("[Scripts]"))
    rune.echo("  /load <path>            - Load Lua script")
    rune.echo("  /reload                 - Reload all scripts")
    rune.echo("  /lua <code>             - Execute Lua code")
    rune.echo("")
    rune.echo(green("[Listing]"))
    rune.echo("  /aliases                - List aliases")
    rune.echo("  /triggers               - List triggers")
    rune.echo("  /timers                 - List timers")
    rune.echo("  /hooks                  - List hooks")
    rune.echo("  /binds                  - List key bindings")
    rune.echo("  /bars                   - List bar renderers")
    rune.echo("  /groups                 - List groups")
    rune.echo("")
    rune.echo(green("[Sending]"))
    rune.echo("  /raw <text>             - Send without alias expansion")
    rune.echo("  /test <line>            - Test triggers with simulated line")
    rune.echo("")
    rune.echo(green("[Other]"))
    rune.echo("  /quit                   - Exit")
    rune.echo("  /help                   - Show this help")
end, "Show available commands")
