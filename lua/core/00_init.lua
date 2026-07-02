-- Core Configuration and Go Primitive Wrappers

-- rune.version is set by Go (single-sourced from the version package,
-- which the telnet TTYPE/MNES responders also report) - data, not API.

rune.config = {
    delimiter = ";"
}

rune.debug = false

function rune.dbg(msg)
    if rune.debug then
        rune.echo(rune.style.gray("[dbg]") .. " " .. msg)
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
    rune.echo(rune.style.red("[" .. label .. "]") .. " error: " .. tostring(result))
    if data.failures >= MAX_CONSECUTIVE_FAILURES and data.enabled then
        data.enabled = false
        rune.echo(rune.style.yellow("[" .. label .. "]") .. " disabled after " ..
            data.failures .. " consecutive errors")
    end
    return false, nil
end

-- Source attribution
-- Returns "file:line" of the script frame `level` levels above the
-- caller (1 = the caller's caller), or nil if unavailable. Registries
-- use this to record which script registered a hook/trigger/alias, so
-- error messages and listings can say whose code is involved.
function rune.caller_source(level)
    local getinfo = debug and debug.getinfo
    if not getinfo then
        return nil
    end
    local info = getinfo(level + 2, "Sl") -- +2 skips this fn and its caller
    if not info or not info.source then
        return nil
    end
    local src = info.source:gsub("^@", "")
    if info.currentline and info.currentline > 0 then
        return src .. ":" .. info.currentline
    end
    return src
end

-- Line objects
-- Server output arrives as objects with :raw() and :clean() methods
-- (raw keeps ANSI codes, clean strips them). rune.line.new builds a
-- compatible object from plain text - used when a handler rewrites a
-- line so the rewritten text flows to the next handler, and by /test.
rune.line = {}

function rune.line.new(raw)
    local clean = nil
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

-- Substitute %1..%N capture references in a template string.
-- Single-pass with greedy digits, so %10 means capture 10, not
-- capture 1 followed by "0". Unknown indices stay literal. The
-- function replacement inserts captured text literally, so "%" in
-- matched text cannot corrupt the template.
function rune.substitute_captures(template, matches)
    return (template:gsub("%%(%d+)", function(d)
        return matches[tonumber(d)]
    end))
end

-- Core function wrappers around Go primitives (rune._*)

-- Send raw text to the server, bypassing alias processing.
-- Echoes send failures (e.g. not connected) rather than raising.
-- Returns true, or nil + error message.
function rune.send_raw(text)
    local ok, err = rune._send_raw(text)
    if not ok then
        rune.echo(rune.style.red("[Error]") .. " " .. tostring(err))
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

-- Client state (read-only view)
-- Go pushes updates into rune._state; rune.state is a read-only proxy
-- so scripts cannot corrupt Go-owned state. Fields: connected,
-- address, scroll_mode, scroll_lines, width, height.
rune.state = setmetatable({}, {
    __index = function(_, key)
        return rune._state[key]
    end,
    __newindex = function()
        error("rune.state is read-only (the client owns this state)", 2)
    end,
})

-- Session store
-- A small Go-owned string store scoped to this client session: it
-- survives /reload (the Lua VM is torn down and rebuilt) but not
-- client exit. Use it for state that must outlive a reload - combat
-- toggles, counters, etc. Values are strings; encode anything richer
-- yourself. State that must survive a restart belongs in rune.store.

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
-- A Go-owned store backed by store.json in rune.config_dir: values
-- survive client exit. Values may be strings, numbers, booleans, or
-- JSON-able tables (all-string keys, or arrays 1..n). Writes hit disk
-- immediately. For state that only needs to outlive /reload, prefer
-- rune.session.

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

-- Input history (Go owns the ring buffer so it survives reloads)

rune.history = {}

function rune.history.get()
    return rune._history.get()
end

function rune.history.add(cmd)
    rune._history.add(cmd)
end

-- UI namespace
-- rune.ui.bar is added by 35_bars.lua, which owns the bar registry.

rune.ui = {}

-- Set the layout configuration.
-- config = { top = {"bar1", {name="pane", height=10}}, bottom = {"input", "status"} }
function rune.ui.layout(config)
    rune._ui.layout(config)
end

-- Force an immediate bar refresh instead of waiting for the ticker.
function rune.ui.refresh_bars()
    rune._ui.refresh_bars()
end

rune.ui.picker = {}

-- Show a picker overlay.
-- opts = {
--   title = "History",              -- optional (modal mode only)
--   items = {"a", "b"} or {{text=..., value=..., desc=...}},
--   on_select = function(value) end,
--   mode = "inline",                -- optional: "inline" or "modal" (default)
--   match_description = true,       -- optional: fuzzy-match descriptions too
-- }
function rune.ui.picker.show(opts)
    rune._ui.picker_show(opts)
end

-- Startup

rune.echo("Rune MUD Client " .. rune.version)
rune.echo("Type /help for commands")
