-- Commands
-- Slash commands: the registry behind /name input, and the commands about
-- the client as a whole (/help, /lua, /echo, /load, /reload, /quit,
-- /version, /groups, /group).
--
-- Pure Lua. Other features add their own commands in their own files
-- (/triggers in triggers.lua, /log in log.lua, ...).
--
-- API:
--   rune.command.add(name, handler, description?, opts?)
--   rune.command.list()            -- {name, description, ...} for /help
--   + shared management (get, enable, disable, toggle, remove, count,
--     clear, remove_group); see registry.lua
--
-- Notes:
--   The command name is the registry name. A handler that keeps failing
--   is disabled on its own; it can never take input dispatch down.

local green, red, yellow, gray = rune.style.green, rune.style.red, rune.style.yellow, rune.style.gray

local registry = rune.registry.new{
    kind = "command",
    label = function(data)
        return 'Command "/' .. data.command .. '"' ..
            (data.source and (" @" .. data.source) or "")
    end,
}

rune.command = registry:expose({})

-- Add a slash command. Returns its handle.
function rune.command.add(name, handler, description, opts)
    if type(name) ~= "string" then
        error("rune.command.add: name must be a string", 2)
    end
    if name == "" or name:find("%s") then
        error("rune.command.add: name must be a non-empty single word", 2)
    end
    if type(handler) ~= "function" then
        error("rune.command.add: handler must be a function", 2)
    end
    if description ~= nil and type(description) ~= "string" then
        error("rune.command.add: description must be a string", 2)
    end
    return registry:add({
        command = name,
        action = handler,
        description = description or "",
        source = rune.caller_source(1),
    }, rune.registry.keyed_opts(name, opts, "rune.command.add"), "rune.command.add")
end

-- List all commands for the picker and /help.
-- Returns array of {name, description, enabled, group, source}.
function rune.command.list()
    local result = registry:list(function(row, data)
        row.name = data.command
        row.description = data.description
    end)
    table.sort(result, function(a, b) return a.name < b.name end)
    return result
end

-- INTERNAL: run a command protected (called by the input dispatcher).
-- Returns true if the name was a known command, even when it is
-- disabled or its handler failed - the input is consumed either way.
function rune.command.dispatch(name, args)
    local data = registry:entry(name)
    if not data then
        return false
    end
    if not registry:active(data) then
        rune.echo(red("[Error]") .. " /" .. name .. " is disabled" ..
            " (re-enable with rune.command.enable)")
        return true
    end
    registry:call(data, args)
    return true
end

-- Defaults

-- /load <path> - Load a Lua script
rune.command.add("load", function(args)
    if args == "" then
        rune.echo("[Usage] /load <path>")
        return
    end
    -- Success is announced by the notice-loaded hook, which also covers
    -- rune.load() from scripts; only the failure is reported here.
    local ok, err = rune.load(args)
    if not ok then
        rune.echo(red("[Error]") .. " " .. tostring(err))
    end
end, "Load a Lua script file")

-- /reload - Clear state and reload init.lua
rune.command.add("reload", function()
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
            rune.echo(red("[Error]") .. " " .. tostring(result))
        end
    else
        rune.echo(red("[Error]") .. " " .. tostring(err))
    end
end, "Execute Lua code")

-- /groups - List all groups and their enabled state
rune.command.add("groups", function()
    local groups = rune.group.list()
    rune.echo(green("[Groups]") .. gray(" (" .. #groups .. " total)"))
    if #groups == 0 then
        rune.echo("  " .. gray("(none)"))
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
            (known and "" or gray("  (no items in this group yet)")))
    else
        rune.group.disable(name)
        rune.echo(yellow("[Group]") .. " " .. name .. " disabled" ..
            (known and "" or gray("  (no items in this group yet)")))
    end
end, "Enable/disable a group (/group <name> on|off)")

-- /echo <text> - Print to the local screen (never sent to the server).
-- Handy for testing and for use in alias/bind command strings.
rune.command.add("echo", function(args)
    rune.echo(args)
end, "Print text locally")

-- /version - Client version
rune.command.add("version", function()
    rune.echo("Rune " .. rune.version .. " (lua: " .. rune.engine .. ")")
end, "Show client version")

-- /quit - Exit the client
rune.command.add("quit", function()
    rune.echo("[System] Goodbye!")
    rune.quit()
end, "Exit the client")

-- /help - Show available commands, generated from the registry so
-- user-added commands appear automatically and descriptions cannot
-- drift from what the picker shows.
rune.command.add("help", function()
    local cmds = rune.command.list()
    rune.echo(green("[Commands]") .. gray(" (" .. #cmds .. " total)"))
    for _, c in ipairs(cmds) do
        local status = c.enabled and "" or (" " .. red("[off]"))
        rune.echo(string.format("  %-12s %s%s",
            "/" .. c.name, c.description, status))
    end
end, "Show available commands")
