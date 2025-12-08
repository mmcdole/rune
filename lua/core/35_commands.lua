-- Slash Command System

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
    rune.echo("[Aliases]")
    if #aliases == 0 then
        rune.echo("  (none)")
        return
    end
    for _, a in ipairs(aliases) do
        local status = a.enabled and "on" or "off"
        local group_str = a.group and (" <" .. a.group .. ">") or ""
        local once_str = a.once and " (once)" or ""
        rune.echo(string.format("  [%s] %s: %s -> %s%s%s",
            status, a.mode, a.match, a.value, group_str, once_str))
    end
end, "List all aliases")

-- /triggers - List all triggers
rune.command.add("triggers", function(args)
    local triggers = rune.trigger.list()
    rune.echo("[Triggers]")
    if #triggers == 0 then
        rune.echo("  (none)")
        return
    end
    for _, t in ipairs(triggers) do
        local status = t.enabled and "on" or "off"
        local group_str = t.group and (" <" .. t.group .. ">") or ""
        local gag_str = t.gag and " (gag)" or ""
        local once_str = t.once and " (once)" or ""
        rune.echo(string.format("  [%s] %s: /%s/ -> %s%s%s%s",
            status, t.mode, t.pattern, t.value, group_str, gag_str, once_str))
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

-- /rmtrigger <name> - Remove a trigger by name
rune.command.add("rmtrigger", function(args)
    if args == "" then
        rune.echo("[Usage] /rmtrigger <name>")
        return
    end

    if rune.trigger.remove(args) then
        rune.echo("[Removed] trigger: " .. args)
    else
        rune.echo("[Error] No trigger named: " .. args)
    end
end, "Remove a trigger by name")

-- /rmalias <name> - Remove an alias by name
rune.command.add("rmalias", function(args)
    if args == "" then
        rune.echo("[Usage] /rmalias <name>")
        return
    end

    if rune.alias.remove(args) then
        rune.echo("[Removed] alias: " .. args)
    else
        rune.echo("[Error] No alias named: " .. args)
    end
end, "Remove an alias by name")

-- /quit - Exit the client
rune.command.add("quit", function(args)
    rune.echo("[System] Goodbye!")
    rune.quit()
end, "Exit the client")

-- /help - Show available commands
rune.command.add("help", function(args)
    rune.echo("[Connection]")
    rune.echo("  /connect <host> <port>  - Connect to server")
    rune.echo("  /disconnect             - Disconnect")
    rune.echo("  /reconnect              - Reconnect to last server")
    rune.echo("")
    rune.echo("[Scripts]")
    rune.echo("  /load <path>            - Load Lua script")
    rune.echo("  /reload                 - Reload all scripts")
    rune.echo("  /lua <code>             - Execute Lua code")
    rune.echo("")
    rune.echo("[Debug]")
    rune.echo("  /aliases                - List aliases")
    rune.echo("  /triggers               - List triggers")
    rune.echo("  /rmtrigger <name>       - Remove trigger by name")
    rune.echo("  /rmalias <name>         - Remove alias by name")
    rune.echo("  /test <line>            - Test triggers")
    rune.echo("")
    rune.echo("[Other]")
    rune.echo("  /quit                   - Exit")
    rune.echo("  /help                   - Show this help")
    rune.echo("")
    rune.echo("[Lua API - Aliases]")
    rune.echo("  rune.alias.exact(key, action, opts?)")
    rune.echo("  rune.alias.regex(pat, action, opts?)")
    rune.echo("")
    rune.echo("[Lua API - Triggers]")
    rune.echo("  rune.trigger.exact(line, action, opts?)")
    rune.echo("  rune.trigger.contains(substr, action, opts?)")
    rune.echo("  rune.trigger.regex(pat, action, opts?)")
    rune.echo("")
    rune.echo("[Lua API - Options]")
    rune.echo("  {name='n', group='g', once=true, priority=50, gag=true}")
    rune.echo("")
    rune.echo("[Lua API - Management]")
    rune.echo("  handle:disable() / handle:enable() / handle:remove()")
    rune.echo("  rune.trigger.disable(name) / enable(name) / remove(name)")
    rune.echo("  rune.alias.remove_group(group) / rune.trigger.remove_group(group)")
    rune.echo("  rune.group.disable(name) / enable(name)")
    rune.echo("")
    rune.echo("[Lua API - Sending]")
    rune.echo("  rune.send(cmd)  - Through alias expansion")
    rune.echo("  rune.send_raw(cmd)  - Direct to socket")
end, "Show available commands")
