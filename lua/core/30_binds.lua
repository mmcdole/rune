-- Key Binding System
-- Built on rune.registry (15_registry.lua), so binds get the same
-- names, groups, source attribution, quarantine, and listings as
-- hooks/triggers/aliases/timers.
--
-- API:
--   rune.bind(key, callback, opts?)  -- Bind a key ("ctrl+r", "f1", "j")
--   rune.unbind(key)                 -- Remove a binding
--   rune.binds.get(key)              -- The binding's handle, or nil
--   rune.binds.list()                -- For /binds
--
-- The key is the registry name, so rune.binds.disable("ctrl+g") and
-- rune.binds.get("ctrl+g") address a bind by the same string you bound.
-- Options: group (see 15_registry.lua). A disabled bind (or one in a
-- disabled group) swallows its key without running the callback.
--
-- Go's role is transport only: the UI forwards keys present in
-- rune.binds._keys(), and rune.binds._dispatch(key) runs the callback.

local by_key = {} -- key -> data, the dispatch index

local registry = rune.registry.new{
    kind = "bind",
    action_field = "callback",
    on_add = function(data)
        -- Rebinding a key replaces the old binding through the registry's
        -- name upsert, which has already removed it by the time we get here.
        by_key[data.key] = data
        rune._ui.presentation_changed()
    end,
    on_remove = function(data)
        if by_key[data.key] == data then
            by_key[data.key] = nil
        end
        rune._ui.presentation_changed()
    end,
}

rune.binds = {}

-- Bind a key to a callback. Returns a handle.
function rune.bind(key, callback, opts)
    if type(key) ~= "string" or key == "" then
        error("rune.bind: key must be a non-empty string", 2)
    end
    if type(callback) ~= "function" then
        error("rune.bind: callback must be a function", 2)
    end
    return registry:add({
        key = key,
        callback = callback,
        source = rune.caller_source(1),
    }, rune.registry.keyed_opts(key, opts, "rune.bind"))
end

-- Remove a binding by key. Returns true if one existed.
function rune.unbind(key)
    return registry:remove(key)
end

-- INTERNAL: called by Go when a bound key is pressed.
-- Returns true if the key had an active binding.
function rune.binds._dispatch(key)
    local data = by_key[key]
    if not data or not registry:active(data) then
        return false
    end

    local label = 'Bind "' .. key .. '"' ..
        (data.source and (" @" .. data.source) or "")
    rune.guarded_call(label, data, data.callback)
    return true
end

-- INTERNAL: Go pulls the bound key set so the UI knows which keys to
-- forward instead of feeding them to the input widget.
function rune.binds._keys()
    local keys = {}
    for key in pairs(by_key) do
        keys[#keys + 1] = key
    end
    return keys
end

-- Management by key (the key is the name)
function rune.binds.get(name)
    return registry:get(name)
end

function rune.binds.disable(name)
    return registry:disable(name)
end

function rune.binds.enable(name)
    return registry:enable(name)
end

function rune.binds.remove(name)
    return registry:remove(name)
end

-- List all binds - returns array of {key, name, group, enabled, source}
function rune.binds.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
        table.insert(result, {
            key = data.key,
            name = data.name,
            group = data.group,
            enabled = data.enabled,
            source = data.source,
        })
    end
    table.sort(result, function(a, b) return a.key < b.key end)
    return result
end

function rune.binds.count()
    return registry:count()
end

function rune.binds.clear()
    registry:clear()
end

function rune.binds.remove_group(group_name)
    return registry:remove_group(group_name)
end
