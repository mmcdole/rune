-- Session Logging
-- Go owns the file handle (rune._log), so an active log survives
-- /reload and is closed on exit. This module owns the policy: WHAT
-- gets written, ANSI stripping, and the header/footer lines.
--
-- Policy: server output lines (after triggers - rewrites are logged as
-- rewritten, gagged lines are not logged) and the local echo of typed
-- input, both ANSI-stripped, so the log reads like the screen. Prompts
-- are skipped (in unterminated mode they repeat on every flush).
-- Client chrome (rune.echo, /help output) is not logged. Register your
-- own hooks against rune._log.write for a different policy; echo does
-- not fire while the server hides input (passwords stay out of logs).

local green, red, dim = rune.style.green, rune.style.red, rune.style.gray

rune.log = {}

local function default_path()
    return rune.config_dir .. "/logs/" .. os.date("%Y-%m-%d_%H-%M-%S") .. ".log"
end

local function stamp(what)
    rune._log.write("--- Log " .. what .. " " .. os.date("%Y-%m-%d %H:%M:%S") .. " ---")
end

-- Start logging. path defaults to config_dir/logs/<timestamp>.log.
-- Returns the resolved path, or nil + error message.
function rune.log.start(path)
    if path == nil or path == "" then
        path = default_path()
    end
    local resolved, err = rune._log.start(path)
    if not resolved then
        return nil, err
    end
    stamp("started")
    return resolved
end

-- Stop logging. Returns true if a log was open.
function rune.log.stop()
    if not rune._log.status() then
        return false
    end
    stamp("stopped")
    return rune._log.stop()
end

-- Returns the active log path, or nil.
function rune.log.status()
    return rune._log.status()
end

-- Append one line to the log directly. No-op while no log is open.
-- For custom policies: disable the default hooks and write your own.
function rune.log.write(text)
    rune._log.write(text)
end

-- Priority 200: runs after the core trigger handlers (priority 100),
-- so the log sees the final rewritten text and never sees gagged
-- lines. rune._log.write is a no-op while no log is open.
rune.hooks.on("output", function(line)
    rune._log.write(line:clean())
end, { name = "log-output", priority = 200 })

rune.hooks.on("echo", function(text)
    rune._log.write(rune._strip_ansi(text))
end, { name = "log-echo", priority = 200 })

-- /log - registered here rather than in 55_commands.lua so the whole
-- logging feature lives in one file.
rune.command.add("log", function(args)
    local sub, rest = args:match("^(%S*)%s*(.*)$")
    if sub == "" or sub == "status" then
        local path = rune.log.status()
        if path then
            rune.echo(green("[Log]") .. " logging to " .. path)
        else
            rune.echo(dim("[Log] not logging") ..
                "  (/log start [file], /log stop)")
        end
    elseif sub == "start" then
        local current = rune.log.status()
        if current then
            rune.echo("[Log] already logging to " .. current ..
                " (/log stop first)")
            return
        end
        local path, err = rune.log.start(rest ~= "" and rest or nil)
        if path then
            rune.echo(green("[Log]") .. " logging to " .. path)
        else
            rune.echo(red("[Error]") .. " " .. tostring(err))
        end
    elseif sub == "stop" then
        local path = rune.log.status()
        if rune.log.stop() then
            rune.echo(green("[Log]") .. " stopped (" .. path .. ")")
        else
            rune.echo(dim("[Log] not logging"))
        end
    else
        rune.echo("[Usage] /log [status] | /log start [file] | /log stop")
    end
end, "Log the session to a file (/log start [file], /log stop)")
