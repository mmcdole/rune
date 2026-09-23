-- Key Binding System
-- Built on rune.registry (20_registry.lua), so binds get the same
-- names, groups, source attribution, quarantine, and listings as
-- hooks/triggers/aliases/timers.
--
-- API:
--   rune.bind(key, action, opts?)  -- Bind a key ("ctrl+r", "f1", "j")
--   rune.unbind(address)           -- Remove a binding
--   rune.binds.get(address)        -- The binding's handle, or nil
--   rune.binds.list()              -- For /binds
--
-- A bind is addressed by its key and, when opts.name is given, by that
-- name too: rune.binds.disable("ctrl+g") and rune.binds.disable("map")
-- reach the same bind. Rebinding a key replaces the bind on it. Options:
-- name, group (see 20_registry.lua). Once the UI routes a key to a
-- disabled bind (or disabled group), it is consumed without a callback.
--
-- Named editor actions run in the UI. Callbacks round-trip through Session.
-- Both use the same registry and presentation snapshot.

local function presentation_changed()
    rune._ui.presentation_changed()
end

local registry = rune.registry.new{
    kind = "bind",
    action_field = "action",
    on_add = presentation_changed,
    on_remove = presentation_changed,
}

rune.binds = {}

local editor_actions = {
    ["input.submit"] = true,
    ["input.newline"] = true,
    ["input.toggle_mode"] = true,
    ["input.cancel"] = true,
    ["input.open_editor"] = true,
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
    -- Validate options and conflicts before changing any assignments.
    if opts ~= nil and type(opts) ~= "table" then
        error("rune.bind: opts must be a table", 2)
    end
    if opts and opts.name ~= nil and #keys > 1 then
        error("rune.bind: a name addresses one bind, not an array of keys", 2)
    end
    for _, k in ipairs(keys) do
        local _, err = registry:resolve(k, opts and opts.name)
        if err then
            error("rune.bind: " .. err, 2)
        end
    end
    local handles = {}
    for _, k in ipairs(keys) do
        handles[#handles + 1] = registry:add({
            key = k,
            action = action,
            source = rune.caller_source(1),
        }, opts)
    end
    return type(key) == "table" and handles or handles[1]
end

-- Remove a binding by key or name. Returns true if one existed.
function rune.unbind(address)
    return registry:remove(address)
end

-- INTERNAL: called by Go when a bound key is pressed.
-- Returns true if the key had an active callback binding.
function rune.binds._dispatch(key)
    local handle = registry:get(key)
    local data = handle and handle._data
    if not data or data.key ~= key or not registry:active(data) or
        type(data.action) ~= "function" then
        return false
    end

    local label = 'Bind "' .. key .. '"' ..
        (data.source and (" @" .. data.source) or "")
    rune.guarded_call(label, data, data.action)
    return true
end

-- Include inactive entries so disabled keys are consumed, but not advertised.
function rune.binds._bindings()
    local bindings = {}
    for _, data in ipairs(registry:items()) do
        bindings[data.key] = {
            action = type(data.action) == "string" and data.action or "",
            enabled = registry:active(data),
            order = data.id,
        }
    end
    return bindings
end

-- Management by key or name
function rune.binds.get(address)
    return registry:get(address)
end

function rune.binds.disable(address)
    return registry:disable(address)
end

function rune.binds.enable(address)
    return registry:enable(address)
end

function rune.binds.remove(address)
    return registry:remove(address)
end

-- List all binds - returns array of {key, name, group, enabled, source}
function rune.binds.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
        table.insert(result, {
            key = data.key,
            action = type(data.action) == "string" and data.action or nil,
            name = data._handle:name(),
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

-- Defaults are ordinary bindings, installed before user configuration.
rune.bind("enter", "input.submit")
rune.bind({"ctrl+j", "shift+enter", "ctrl+enter"}, "input.newline")
rune.bind("alt+v", "input.toggle_mode")

rune.bind("esc", "input.cancel")
rune.bind("ctrl+e", "input.open_editor")
