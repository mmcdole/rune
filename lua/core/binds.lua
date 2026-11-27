-- Binds
-- Key bindings: a key runs a function, or a named input action the UI
-- performs itself (input.submit, input.newline, ...).
--
-- Go routes keys: named actions run in the UI, callbacks round-trip to
-- rune.binds._dispatch. This file owns the registry and the defaults.
--
-- API:
--   rune.bind(key or {keys}, action, opts?)  -- returns a handle (or handles)
--   rune.unbind(key)
--   rune.binds.list()                       -- {key, action, ...} by key
--   + shared management by key (get, enable, disable, toggle, remove,
--     count, clear, remove_group); see registry.lua
--
-- Notes:
--   The key is the registry name, so rune.binds.disable("ctrl+g") names the
--   bind you made. A key routed to a disabled bind is consumed without a
--   callback. clear() restores the default keys.

local green, red, yellow, cyan, gray = rune.style.green, rune.style.red, rune.style.yellow, rune.style.cyan, rune.style.gray

local registry = rune.registry.new{
    kind = "bind",
    on_change = rune._ui.binds_changed,
}

rune.binds = registry:expose({})

-- Actions the UI runs itself; anything else must be a function.
local editor_actions = {
    ["input.submit"] = true,
    ["input.newline"] = true,
    ["input.toggle_mode"] = true,
    ["input.cancel"] = true,
    ["input.open_editor"] = true,
    ["input.delete_word"] = true,
    ["input.word_left"] = true,
    ["input.word_right"] = true,
}

-- An array is shorthand for independent bindings; returns an array of handles.
function rune.bind(key, action, opts)
    if type(action) ~= "function" and not editor_actions[action] then
        error("rune.bind: expected a callback or known input action", 2)
    end
    local keys = type(key) == "table" and key or {key}
    local count = 0
    for index, value in pairs(keys) do
        if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or
            type(value) ~= "string" or value == "" then
            error("rune.bind: expected a key or non-empty array of keys", 2)
        end
        count = count + 1
    end
    if count == 0 or count ~= #keys then
        error("rune.bind: expected a key or non-empty array of keys", 2)
    end
    -- Validate options before changing any assignments.
    if opts ~= nil and type(opts) ~= "table" then
        error("rune.bind: opts must be a table", 2)
    end
    local handles = {}
    for _, name in ipairs(keys) do
        handles[#handles + 1] = registry:add({
            key = name,
            action = action,
            source = rune.caller_source(1),
        }, rune.registry.keyed_opts(name, opts, "rune.bind"), "rune.bind")
    end
    return type(key) == "table" and handles or handles[1]
end

-- Remove a binding by key. Returns true if one existed.
function rune.unbind(key)
    return registry:remove(key)
end

-- List all binds - returns array of {key, action, name, group, enabled, source, core}
function rune.binds.list()
    local result = registry:list(function(row, data)
        row.key = data.key
        row.action = type(data.action) == "string" and data.action or nil
    end)
    table.sort(result, function(a, b) return a.key < b.key end)
    return result
end

-- INTERNAL: called by Go when a bound key is pressed.
-- Returns true if the key had an active callback binding.
function rune.binds._dispatch(key)
    local data = registry:entry(key)
    if not data or not registry:active(data) or type(data.action) ~= "function" then
        return false
    end
    registry:call(data)
    return true
end

-- Include inactive entries so disabled keys are consumed, but not advertised.
function rune.binds._bindings()
    local bindings = {}
    for _, data in ipairs(registry:snapshot()) do
        bindings[data.key] = {
            action = type(data.action) == "string" and data.action or "",
            enabled = registry:active(data),
            order = data.id,
        }
    end
    return bindings
end

-- Defaults
-- Ordinary bindings, installed before user configuration.
rune.bind("enter", "input.submit")
rune.bind({"ctrl+j", "shift+enter", "ctrl+enter"}, "input.newline")
rune.bind("alt+v", "input.toggle_mode")

rune.bind("esc", "input.cancel")
rune.bind("ctrl+e", "input.open_editor")

-- Most terminals send ctrl+backspace as ctrl+h, so that combination
-- cannot be bound distinctly.
rune.bind({"ctrl+w", "alt+backspace"}, "input.delete_word")
rune.bind({"alt+left", "ctrl+left"}, "input.word_left")
rune.bind({"alt+right", "ctrl+right"}, "input.word_right")

-- /binds - List all key bindings
rune.command.add("binds", function()
    local binds = rune.binds.list()
    rune.echo(green("[Binds]") .. gray(" (" .. #binds .. " total)"))
    if #binds == 0 then
        rune.echo("  " .. gray("(none)"))
        return
    end
    -- The key is the name, so there is no separate name column to print.
    for _, b in ipairs(binds) do
        local status = b.enabled and green("[on] ") or red("[off]")
        local group_str = b.group and ("  " .. cyan("<" .. b.group .. ">")) or ""
        local src_str = rune.registry._origin(b)
        rune.echo(string.format("  %s %-16s%s%s",
            status, yellow(b.key), group_str, src_str))
    end
end, "List all key bindings")
