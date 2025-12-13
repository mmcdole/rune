-- Slash Command System

-- ANSI color helpers for command output
local function green(s) return "\027[32m" .. s .. "\027[0m" end
local function red(s) return "\027[31m" .. s .. "\027[0m" end
local function yellow(s) return "\027[33m" .. s .. "\027[0m" end
local function cyan(s) return "\027[36m" .. s .. "\027[0m" end
local function dim(s) return "\027[90m" .. s .. "\027[0m" end

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

-- Track last connection for reconnect
local last_host = nil
local last_port = nil

-- /connect <host> <port> - Connect to server
rune.command.add("connect", function(args)
    local host, port = args:match("^(%S+)%s+(%d+)$")
    if host and port then
        last_host = host
        last_port = port
        rune.connect(host .. ":" .. port)
    else
        rune.echo("[Usage] /connect <host> <port>")
    end
end, "Connect to a MUD server")

-- /disconnect - Disconnect from server
rune.command.add("disconnect", function(args)
    rune.disconnect()
end, "Disconnect from server")

-- /reconnect - Reconnect to last server
rune.command.add("reconnect", function(args)
    if last_host and last_port then
        rune.connect(last_host .. ":" .. last_port)
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
    local err = rune.load(args)
    if err then
        rune.echo("[Error] " .. err)
    else
        rune.echo("[Loaded] " .. args)
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
        rune.echo(string.format("  %s %-8s %s %s %s%s%s",
            status, a.mode, yellow('"' .. a.match .. '"'), dim("->"), a.value, group_str, flags_str))
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
        rune.echo(string.format("  %s %-8s %s %s %s%s%s",
            status, t.mode, yellow('"' .. t.match .. '"'), dim("->"), t.value, group_str, flags_str))
    end
end, "List all triggers")

-- /test <line> - Simulate server output (test triggers)
rune.command.add("test", function(args)
    if args == "" then
        rune.echo("[Usage] /test <line>")
        return
    end

    rune.echo("[Test Input] " .. args)

    -- Create a mock Line object for testing
    local mock_line = {
        _raw = args,
        _clean = args,
        raw = function(self) return self._raw end,
        clean = function(self) return self._clean end,
    }

    local modified, show = rune.trigger.process(mock_line)
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
        local timing = string.format("%s %.1fs", t.mode, t.seconds)
        rune.echo(string.format("  %s %-12s %s %s%s%s",
            status, timing, dim("->"), t.value, group_str, name_str))
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
        rune.echo(string.format("  %s %-12s %s %s %s%s",
            status, h.event, pri_str, dim("->"), name_str, group_str))
    end
end, "List all hooks")

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
