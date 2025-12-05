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

-- Startup

rune.print("Rune MUD Client")
rune.print("Type /help for commands")
