-- Timer and Async APIs

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
