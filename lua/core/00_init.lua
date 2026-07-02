-- Core Configuration and Go Primitive Wrappers

rune.config = {
    delimiter = ";"
}

rune.debug = false

function rune.dbg(msg)
    if rune.debug then
        rune.echo("\027[90m[dbg]\027[0m " .. msg)
    end
end

-- Failure quarantine
-- Callbacks that keep failing are disabled automatically so one broken
-- script cannot spam errors on every line, keypress, or timer tick.

local MAX_CONSECUTIVE_FAILURES = 3

-- Run a callback protected, tracking consecutive failures on `data`
-- (any registry entry with an `enabled` field). Errors are echoed under
-- `label`; when the failure limit is reached the entry is disabled and
-- the user notified (re-enable with the registry's enable function).
-- Returns ok, result from the callback.
function rune.guarded_call(label, data, fn, ...)
    local ok, result = pcall(fn, ...)
    if ok then
        data.failures = nil
        return true, result
    end

    data.failures = (data.failures or 0) + 1
    rune.echo("\027[31m[" .. label .. "]\027[0m error: " .. tostring(result))
    if data.failures >= MAX_CONSECUTIVE_FAILURES and data.enabled then
        data.enabled = false
        rune.echo("\027[33m[" .. label .. "]\027[0m disabled after " ..
            data.failures .. " consecutive errors")
    end
    return false, nil
end

-- Core function wrappers around Go primitives (rune._*)

-- Send raw text to the server, bypassing alias processing.
-- Echoes send failures (e.g. not connected) rather than raising.
-- Returns true, or nil + error message.
function rune.send_raw(text)
    local ok, err = rune._send_raw(text)
    if not ok then
        rune.echo("\027[31m[Error]\027[0m " .. tostring(err))
    end
    return ok, err
end

function rune.echo(text)
    rune._echo(text)
end

function rune.quit()
    rune._quit()
end

function rune.connect(address)
    rune._connect(address)
end

function rune.disconnect()
    rune._disconnect()
end

function rune.reload()
    rune._reload()
end

-- Load a Lua script. Returns true, or nil + error message.
function rune.load(path)
    return rune._load(path)
end

-- Startup

rune.echo("Rune MUD Client")
rune.echo("Type /help for commands")
