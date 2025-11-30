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
        rune.print("[Usage] /connect <host> <port>")
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
        rune.print("[Error] No previous connection")
    end
end, "Reconnect to last server")

-- /load <path> - Load a Lua script
rune.command.add("load", function(args)
    if args == "" then
        rune.print("[Usage] /load <path>")
        return
    end
    local err = rune.load(args)
    if err then
        rune.print("[Error] " .. err)
    else
        rune.print("[Loaded] " .. args)
    end
end, "Load a Lua script file")

-- /reload - Clear state and reload init.lua
rune.command.add("reload", function(args)
    rune.reload()
end, "Reload all scripts")

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
end, "Execute Lua code")

-- /aliases - List all aliases
rune.command.add("aliases", function(args)
    rune.alias.list()
end, "List all aliases")

-- /triggers - List all triggers
rune.command.add("triggers", function(args)
    rune.trigger.list()
end, "List all triggers")

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
end, "Test triggers with simulated line")

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
end, "Remove a trigger by ID or pattern")

-- /quit - Exit the client
rune.command.add("quit", function(args)
    rune.print("[System] Goodbye!")
    rune.quit()
end, "Exit the client")

-- /help - Show available commands
rune.command.add("help", function(args)
    rune.print("[Connection]")
    rune.print("  /connect <host> <port>  - Connect to server")
    rune.print("  /disconnect             - Disconnect")
    rune.print("  /reconnect              - Reconnect to last server")
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
    rune.print("  /quit                   - Exit")
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
end, "Show available commands")

-- Populate the "commands" pane for the slash picker
-- This function writes all commands to a hidden pane that the TUI filters
local function populate_commands_pane()
    rune.pane.create("commands")
    rune.pane.clear("commands")
    for _, cmd in ipairs(rune.command.list()) do
        local line = "/" .. cmd.name
        if cmd.description and cmd.description ~= "" then
            line = line .. " - " .. cmd.description
        end
        rune.pane.write("commands", line)
    end
end

-- Register to populate commands on ready (after all scripts loaded)
rune.hooks.register("ready", populate_commands_pane, { priority = 50 })

-- Also repopulate after reload
rune.hooks.register("reloaded", populate_commands_pane, { priority = 50 })
