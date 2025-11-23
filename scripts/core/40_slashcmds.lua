-- Slash Command System

rune.command = {}
local commands = {}  -- Private storage

-- Add a slash command
function rune.command.add(name, handler)
    commands[name] = handler
end

-- Get a slash command handler
function rune.command.get(name)
    return commands[name]
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
        rune.print("[Usage] /connect <host> <port>")
    end
end)

-- /disconnect or /dc - Disconnect from server
rune.command.add("disconnect", function(args)
    rune.disconnect()
end)
rune.command.add("dc", rune.command.get("disconnect"))

-- /reconnect or /rc - Reconnect to last server
rune.command.add("reconnect", function(args)
    if last_host and last_port then
        rune.connect(last_host .. ":" .. last_port)
    else
        rune.print("[Error] No previous connection")
    end
end)
rune.command.add("rc", rune.command.get("reconnect"))

-- /load <path> - Load a Lua script
rune.command.add("load", function(args)
    if args == "" then
        rune.print("[Usage] /load <path>")
        return
    end
    rune.load(args)
end)

-- /reload - Clear state and reload init.lua
rune.command.add("reload", function(args)
    rune.reload()
end)

-- /lua <code> - Execute Lua code directly
rune.command.add("lua", function(args)
    if args == "" then
        rune.print("[Usage] /lua <code>")
        return
    end

    local fn, err = loadstring(args)
    if fn then
        local ok, result = pcall(fn)
        if ok then
            if result ~= nil then
                rune.print(tostring(result))
            end
        else
            rune.print("[Error] " .. tostring(result))
        end
    else
        rune.print("[Error] " .. tostring(err))
    end
end)

-- /aliases - List all aliases
rune.command.add("aliases", function(args)
    rune.alias.list()
end)

-- /triggers - List all triggers
rune.command.add("triggers", function(args)
    rune.trigger.list()
end)

-- /test <line> - Simulate server output (test triggers)
rune.command.add("test", function(args)
    if args == "" then
        rune.print("[Usage] /test <line>")
        return
    end

    rune.print("[Test Input] " .. args)
    local modified, show = rune.trigger.process(args)
    if show and modified ~= "" then
        rune.print("[Test Output] " .. modified)
    else
        rune.print("[Test Output] (gagged)")
    end
end)

-- /rmtrigger <id or pattern> - Remove a trigger
rune.command.add("rmtrigger", function(args)
    if args == "" then
        rune.print("[Usage] /rmtrigger <id or pattern>")
        return
    end

    -- Try to parse as number first
    local id = tonumber(args)
    if id then
        rune.trigger.remove(id)
    else
        rune.trigger.remove(args)
    end
end)

-- /quit or /q - Exit the client
rune.command.add("quit", function(args)
    rune.print("[System] Goodbye!")
    rune.quit()
end)
rune.command.add("q", rune.command.get("quit"))

-- /help - Show available commands
rune.command.add("help", function(args)
    rune.print("[Connection]")
    rune.print("  /connect <host> <port>  - Connect to server")
    rune.print("  /disconnect, /dc        - Disconnect")
    rune.print("  /reconnect, /rc         - Reconnect to last server")
    rune.print("")
    rune.print("[Scripts]")
    rune.print("  /load <path>            - Load Lua script")
    rune.print("  /reload                 - Reload all scripts")
    rune.print("  /lua <code>             - Execute Lua code")
    rune.print("")
    rune.print("[Debug]")
    rune.print("  /aliases                - List aliases")
    rune.print("  /triggers               - List triggers")
    rune.print("  /rmtrigger <id>         - Remove trigger")
    rune.print("  /test <line>            - Test triggers")
    rune.print("")
    rune.print("[Other]")
    rune.print("  /quit, /q               - Exit")
    rune.print("  /help                   - Show this help")
    rune.print("")
    rune.print("[Lua API]")
    rune.print("  rune.alias.add(name, expansion_or_func)")
    rune.print("  rune.alias.remove(name)")
    rune.print("  rune.trigger.add(pattern, action, {gag=bool})")
    rune.print("  rune.trigger.remove(id_or_pattern)")
    rune.print("  rune.trigger.enable(id_or_pattern, bool)")
    rune.print("  rune.timer.after(seconds, func)")
    rune.print("  rune.timer.every(seconds, func)")
end)
