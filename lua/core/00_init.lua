-- Core Configuration and Go Primitive Wrappers
-- This file establishes the foundation: config and all rune._* wrapper functions

-- =============================================================================
-- CONFIGURATION
-- =============================================================================

rune.config = {
    delimiter = ";"
}

-- =============================================================================
-- CORE FUNCTIONS
-- =============================================================================

-- rune.send_raw(text): Bypass alias processing, write directly to socket
function rune.send_raw(text)
    rune._send_raw(text)
end

-- rune.print(text): Output text to local display
function rune.print(text)
    rune._print(text)
end

-- rune.quit(): Exit the client
function rune.quit()
    rune._quit()
end

-- rune.connect(address): Connect to server
function rune.connect(address)
    rune._connect(address)
end

-- rune.disconnect(): Disconnect from server
function rune.disconnect()
    rune._disconnect()
end

-- rune.reload(): Reload all scripts
function rune.reload()
    rune._reload()
end

-- rune.load(path): Load a Lua script
function rune.load(path)
    rune._load(path)
end

-- =============================================================================
-- TIMERS
-- =============================================================================

rune.timer = {}

-- rune.timer.after(seconds, callback): Schedule a one-time delayed callback
function rune.timer.after(seconds, callback)
    rune._timer.after(seconds, callback)
end

-- rune.timer.every(seconds, callback): Schedule a repeating callback
-- Returns: timer ID for cancellation
function rune.timer.every(seconds, callback)
    return rune._timer.every(seconds, callback)
end

-- rune.timer.cancel(id): Cancel a repeating timer by ID
function rune.timer.cancel(id)
    rune._timer.cancel(id)
end

-- rune.timer.cancel_all(): Cancel all repeating timers
function rune.timer.cancel_all()
    rune._timer.cancel_all()
end

-- rune.delay(seconds, action): Convenience wrapper for delayed commands
-- Usage:
--   rune.delay(1.5, "kill orc")
--   rune.delay(2.0, function() rune.print("Done!") end)
function rune.delay(seconds, action)
    rune.timer.after(seconds, function()
        if type(action) == "function" then
            action()
        else
            rune.send(action)
        end
    end)
end

-- =============================================================================
-- REGEX
-- =============================================================================

rune.regex = {}

-- rune.regex.match(pattern, text): Match using Go's regexp (cached)
-- Returns: table of matches [1]=full, [2]=group1, etc. or nil if no match
function rune.regex.match(pattern, text)
    return rune._regex.match(pattern, text)
end

-- =============================================================================
-- UI
-- =============================================================================

-- Status bar
rune.status = {}

function rune.status.set(text)
    rune._status.set(text)
end

-- Panes
rune.pane = {}

function rune.pane.create(name)
    rune._pane.create(name)
end

function rune.pane.write(name, text)
    rune._pane.write(name, text)
end

function rune.pane.toggle(name)
    rune._pane.toggle(name)
end

function rune.pane.clear(name)
    rune._pane.clear(name)
end

function rune.pane.bind(key, name)
    rune._pane.bind(key, name)
end

-- Info bar
rune.infobar = {}

function rune.infobar.set(text)
    rune._infobar.set(text)
end
