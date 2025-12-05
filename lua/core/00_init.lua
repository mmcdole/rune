-- Core Configuration and Go Primitive Wrappers

rune.config = {
    delimiter = ";"
}

rune.debug = false

function rune.dbg(msg)
    if rune.debug then
        rune.print("\027[90m[dbg]\027[0m " .. msg)
    end
end

-- Core function wrappers around Go primitives (rune._*)

function rune.send_raw(text)
    rune._send_raw(text)
end

function rune.print(text)
    rune._print(text)
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

-- Returns nil on success, error string on failure
function rune.load(path)
    return rune._load(path)
end

-- Timer API

rune.timer = {}

function rune.timer.after(seconds, callback)
    rune._timer.after(seconds, callback)
end

-- Returns timer ID for cancellation
function rune.timer.every(seconds, callback)
    return rune._timer.every(seconds, callback)
end

function rune.timer.cancel(id)
    rune._timer.cancel(id)
end

function rune.timer.cancel_all()
    rune._timer.cancel_all()
end

-- Convenience wrapper: rune.delay(1.5, "kill orc") or rune.delay(2.0, function() ... end)
function rune.delay(seconds, action)
    rune.timer.after(seconds, function()
        if type(action) == "function" then
            action()
        else
            rune.send(action)
        end
    end)
end

-- Regex API

rune.regex = {}

-- Returns Regex userdata with :match(text) method, or nil + error
function rune.regex.compile(pattern)
    return rune._regex.compile(pattern)
end

-- Pane API

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

-- Startup

rune.print("Rune MUD Client")
rune.print("Type /help for commands")
