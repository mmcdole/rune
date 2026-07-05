-- Session Logging
-- Go owns the file handle (rune._log), so an active log survives
-- /reload and is closed on exit. This module owns the policy: WHAT
-- gets written, ANSI stripping, and the header/footer lines.
--
-- Policy: server output lines (after triggers - rewrites are logged as
-- rewritten, gagged lines are not logged) and the local echo of typed
-- input. Plain mode (the default) strips ANSI so the log reads like
-- the screen; raw mode (opts.raw / "/log start raw") keeps the codes
-- for color-faithful transcripts (view with `less -R`). Prompts are
-- skipped (in unterminated mode they repeat on every flush). Client
-- chrome (rune.echo, /help output) is not logged. Register your own
-- hooks against rune._log.write for a different policy; echo does not
-- fire while the server hides input (passwords stay out of logs).

local green, red, dim = rune.style.green, rune.style.red, rune.style.gray

rune.log = {}

-- Raw mode keeps ANSI codes in the log. The file handle outlives
-- /reload in Go, so the mode has to as well: it is mirrored into
-- rune.session and restored when this module reloads over an open
-- log - otherwise a raw log would silently turn stripped mid-file.
local raw_mode = false
if rune._log.status() and rune.session.get("log_raw") then
    raw_mode = true
end

local function default_path()
    return rune.config_dir .. "/logs/" .. os.date("%Y-%m-%d_%H-%M-%S") .. ".log"
end

local function stamp(what)
    rune._log.write("--- Log " .. what .. " " .. os.date("%Y-%m-%d %H:%M:%S") .. " ---")
end

-- Start logging. path defaults to config_dir/logs/<timestamp>.log.
-- opts: { raw = true } keeps ANSI codes instead of stripping them.
-- Returns the resolved path, or nil + error message.
function rune.log.start(path, opts)
    if opts ~= nil and type(opts) ~= "table" then
        error("rune.log.start: opts must be a table", 2)
    end
    if path == nil or path == "" then
        path = default_path()
    end
    local resolved, err = rune._log.start(path)
    if not resolved then
        return nil, err
    end
    raw_mode = not not (opts and opts.raw)
    if raw_mode then
        rune.session.set("log_raw", "1")
    else
        rune.session.delete("log_raw")
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
    raw_mode = false
    rune.session.delete("log_raw")
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
    rune._log.write(raw_mode and line:raw() or line:clean())
end, { name = "log-output", priority = 200 })

rune.hooks.on("echo", function(text)
    rune._log.write(raw_mode and text or rune._strip_ansi(text))
end, { name = "log-echo", priority = 200 })

-- /log - registered here rather than in 55_commands.lua so the whole
-- logging feature lives in one file.
rune.command.add("log", function(args)
    local sub, rest = args:match("^(%S*)%s*(.*)$")
    if sub == "" or sub == "status" then
        local path = rune.log.status()
        if path then
            rune.echo(green("[Log]") .. " logging to " .. path ..
                (raw_mode and dim("  (raw)") or ""))
        else
            rune.echo(dim("[Log] not logging") ..
                "  (/log start [raw] [file], /log stop)")
        end
    elseif sub == "start" then
        local current = rune.log.status()
        if current then
            rune.echo("[Log] already logging to " .. current ..
                " (/log stop first)")
            return
        end
        local raw = false
        local file = rest
        local first, remainder = rest:match("^(%S+)%s*(.*)$")
        if first == "raw" then
            raw = true
            file = remainder
        end
        local path, err = rune.log.start(file ~= "" and file or nil, { raw = raw })
        if path then
            rune.echo(green("[Log]") .. " logging to " .. path ..
                (raw and dim("  (raw)") or ""))
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
        rune.echo("[Usage] /log [status] | /log start [raw] [file] | /log stop")
    end
end, "Log the session to a file (/log start [raw] [file], /log stop)")
