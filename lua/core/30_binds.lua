-- Key Binding System
-- Built on rune.registry (20_registry.lua), so binds get the same
-- names, groups, source attribution, quarantine, and listings as
-- hooks/triggers/aliases/timers.
--
-- API:
--   rune.bind(key, action, opts?)  -- Bind a key ("ctrl+r", "f1", "j")
--   rune.unbind(key)                 -- Remove a binding
--   rune.binds.get(key)              -- The binding's handle, or nil
--   rune.binds.list()                -- For /binds
--
-- The key is the registry name, so rune.binds.disable("ctrl+g") and
-- rune.binds.get("ctrl+g") address a bind by the same string you bound.
-- Options: group (see 20_registry.lua). Once the UI routes a key to a
-- disabled bind (or disabled group), it is consumed without a callback.
--
-- Named editor actions run in the UI. Callbacks round-trip through Session.
-- Both use the same registry and presentation snapshot.

local by_key = {} -- key -> data, the dispatch index

local registry = rune.registry.new{
    kind = "bind",
    action_field = "action",
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
        }, rune.registry.keyed_opts(name, opts, "rune.bind"))
    end
    return type(key) == "table" and handles or handles[1]
end

-- Remove a binding by key. Returns true if one existed.
function rune.unbind(key)
    return registry:remove(key)
end

-- INTERNAL: called by Go when a bound key is pressed.
-- Returns true if the key had an active callback binding.
function rune.binds._dispatch(key)
    local data = by_key[key]
    if not data or not registry:active(data) or type(data.action) ~= "function" then
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
    for key, data in pairs(by_key) do
        bindings[key] = {
            action = type(data.action) == "string" and data.action or "",
            enabled = registry:active(data),
            order = data.id,
        }
    end
    return bindings
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
            action = type(data.action) == "string" and data.action or nil,
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

-- Defaults are ordinary bindings, installed before user configuration.
rune.bind("enter", "input.submit")
rune.bind({"ctrl+j", "shift+enter", "ctrl+enter"}, "input.newline")
rune.bind("alt+v", "input.toggle_mode")

rune.bind("esc", "input.cancel")
rune.bind("ctrl+e", "input.open_editor")
