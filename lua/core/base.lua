-- Base
-- The thin Lua faces of things Go owns: configuration, client state,
-- line objects, session and durable storage, and the core actions.
--
-- Go owns every value here (rune._config, rune._state, rune._session,
-- rune._store) and the actions behind them; this file defines the public
-- names, since every public rune.* name lives in Lua.
--
-- API:
--   rune.echo(text)  rune.time()  rune.quit()  rune.reload()  rune.load(path)
--   rune.config.get(key)  rune.config.set(key, value)
--   rune.state.<field>                     -- read-only client state
--   rune.line.new(raw)                     -- line object with :raw()/:clean()
--   rune.session.get/set/delete(key)       -- survives /reload
--   rune.store.get/set/delete(key)         -- survives restarts
--   rune.debug, rune.dbg(msg)
--
-- Notes:
--   rune.version, rune.engine, and rune.config_dir are data set by Go.

-- Configuration
-- Go owns the typed values and defaults.

rune.config = {}

function rune.config.get(key)
    return rune._config.get(key)
end

function rune.config.set(key, value)
    return rune._config.set(key, value)
end

setmetatable(rune.config, {
    __newindex = function()
        error("rune.config: values cannot be assigned directly; use rune.config.set(key, value)", 2)
    end,
})

-- Core actions

function rune.echo(text)
    rune._echo(text)
end

-- Wall-clock seconds since the Unix epoch, with sub-second precision.
-- Subtract two readings for a duration; os.date accepts it for display.
function rune.time()
    return rune._time()
end

function rune.quit()
    rune._quit()
end

function rune.reload()
    rune._reload()
end

-- Load a Lua script. Returns true, or nil + error message.
function rune.load(path)
    return rune._load(path)
end

rune.debug = false

function rune.dbg(msg)
    if rune.debug then
        rune.echo(rune.style.gray("[dbg]") .. " " .. msg)
    end
end

-- Client state
-- Go pushes updates into rune._state; rune.state is a read-only proxy so
-- scripts cannot corrupt it. Fields: connected, address, scroll_mode,
-- scroll_lines, search_active, width, height.

rune.state = setmetatable({}, {
    __index = function(_, key)
        return rune._state[key]
    end,
    __newindex = function()
        error("rune.state: read-only (the client owns this state)", 2)
    end,
})

-- Line objects
-- Server output arrives as objects with :raw() and :clean() methods (raw
-- keeps ANSI codes, clean strips them). rune.line.new builds a compatible
-- object from text, so a handler's rewrite flows on to the next handler.

rune.line = {}

function rune.line.new(raw)
    local clean
    return {
        raw = function() return raw end,
        clean = function()
            if clean == nil then
                clean = rune._strip_ansi(raw)
            end
            return clean
        end,
    }
end

-- Session store
-- Scoped to this client session: survives /reload (the VM is rebuilt)
-- but not exit. Values are strings; encode anything richer yourself.

rune.session = {}

function rune.session.set(key, value)
    rune._session.set(key, value)
end

-- Returns the stored string, or nil if unset.
function rune.session.get(key)
    return rune._session.get(key)
end

function rune.session.delete(key)
    rune._session.delete(key)
end

-- Durable store
-- Backed by store.json in rune.config_dir, so values survive exit. Values
-- may be strings, numbers, booleans, or JSON-able tables. Writes hit disk
-- immediately.

rune.store = {}

-- Returns true, or nil + error message. set(key, nil) deletes.
function rune.store.set(key, value)
    return rune._store.set(key, value)
end

-- Returns the stored value (decoded), or nil if unset.
function rune.store.get(key)
    return rune._store.get(key)
end

function rune.store.delete(key)
    return rune._store.delete(key)
end
