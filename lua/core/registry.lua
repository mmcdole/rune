-- Registries
-- The one factory behind every callback registry: hooks, binds, bars,
-- timers, aliases, triggers, commands, and GMCP handlers. It owns an
-- entry's lifecycle; each module owns only how its entries match and fire.
--
-- Usage:
--   local registry = rune.registry.new{
--       kind = "trigger",          -- names the kind in labels and messages
--       options = { "gag", "raw" }, -- module options beyond the common set
--       index_by = "event",        -- optional: registry.indexed[value] lists
--       label = function(data) end, -- optional: label for errors
--       on_add = function(data) end,
--       on_remove = function(data) end,
--       on_change = function() end,
--   }
--   rune.trigger = registry:expose({})
--   local handle = registry:add(data, opts, "rune.trigger.contains")
--
-- Every entry gets: id (insertion order), enabled, priority (default 50,
-- lower runs first), name (unique; re-adding a name replaces the entry),
-- group (master switch, see rune.group), once, core, and _handle. The
-- callback always lives in data.action.
--
-- Common options: name, group, priority, once. An option outside those
-- and spec.options is ignored with a notice, which catches typos.
--
-- Entry names are kebab-case ("log-output"); Rune's internal entries add a
-- leading underscore ("_trigger-output") and may change between releases.
--
-- Name is the only identity. Registries whose entries have a natural key
-- (a bind's key, a bar's name, a command, an exact alias's phrase) pass it
-- as the name via rune.registry.keyed_opts, so the management functions
-- address what the user typed.
--
-- Lists are replaced, never mutated, so a dispatch loop over snapshot()
-- (or an indexed list) is unaffected by callbacks that add or remove
-- entries; removals are honored through active().
--
-- Core entries: everything registered while the embedded core loads is
-- sealed as a default (rune.registry._seal, called by the loader). clear()
-- removes user entries and restores the defaults.
--
-- on_change runs after anything that can change which entries are active:
-- add, remove, enable, disable, toggle, and a group switch covering one of
-- the registry's entries.
--
-- Handle API: :enable() :disable() :toggle() :remove() :name() :group() :action()

rune.registry = {}

-- Every registry, for group switches and sealing.
local registries = {}

local Handle = {}
Handle.__index = Handle

local function set_enabled(handle, enabled)
    if handle._data.removed or handle._data.enabled == enabled then
        return
    end
    handle._data.enabled = enabled
    local on_change = handle._registry.on_change
    if on_change then
        on_change()
    end
end

-- Enabling also forgives past failures, so a quarantined entry gets a
-- fresh run of chances.
function Handle:enable()
    self._data.failures = nil
    set_enabled(self, true)
    return self
end

function Handle:disable()
    set_enabled(self, false)
    return self
end

function Handle:toggle()
    if self._data.enabled then
        return self:disable()
    end
    return self:enable()
end

function Handle:remove()
    self._registry:_remove_data(self._data)
    return self
end

function Handle:name()
    return self._data.name
end

function Handle:group()
    return self._data.group
end

-- The registered action: a function, or the command string for the
-- string-action forms. Calling it directly bypasses the enabled/group
-- checks and the failure quarantine, so this is for capturing and
-- wrapping an existing entry, not for dispatch.
function Handle:action()
    return self._data.action
end

local Registry = {}
Registry.__index = Registry

-- Build the opts table for a registry whose entries have a natural key,
-- making that key the name.
--
-- A different opts.name is dropped with a notice rather than raised:
-- raising aborts the rest of the user's script, so one stale option in
-- an old init.lua would cost every registration below it.
function rune.registry.keyed_opts(key, opts, label)
    if opts ~= nil and type(opts) ~= "table" then
        error(label .. ": opts must be a table", 3)
    end
    local merged = {}
    if opts then
        for k, v in pairs(opts) do
            merged[k] = v
        end
    end
    if merged.name ~= nil and merged.name ~= key then
        local source = rune.caller_source(2)
        rune.echo(rune.style.yellow("[Deprecated]") .. " " .. label ..
            ": name '" .. tostring(merged.name) .. "' ignored, manage it as '" ..
            key .. "'" .. (source and (" @" .. source) or ""))
    end
    merged.name = key
    return merged
end

local function capitalize(s)
    return (s:gsub("^%l", string.upper))
end

-- The default error label: Kind "name" @source, or Kind #id @source.
local function default_label(title)
    return function(data)
        local who = data.name and ('"' .. data.name .. '"') or ("#" .. data.id)
        return title .. " " .. who .. (data.source and (" @" .. data.source) or "")
    end
end

local COMMON_OPTIONS = { name = "string", group = "string", priority = "number", once = "boolean" }

function rune.registry.new(spec)
    spec = spec or {}
    local kind = spec.kind or "item"
    local allowed = {}
    for key in pairs(COMMON_OPTIONS) do
        allowed[key] = true
    end
    for _, key in ipairs(spec.options or {}) do
        allowed[key] = true
    end
    local registry = setmetatable({
        kind = kind,
        label = spec.label or default_label(capitalize(kind)),
        allowed = allowed,
        index_by = spec.index_by,
        indexed = {},  -- index_by value -> sorted list (replaced, never mutated)
        on_add = spec.on_add,
        on_remove = spec.on_remove,
        on_change = spec.on_change,
        entries = {},  -- all entries, sorted by (priority, id); replaced, never mutated
        by_name = {},  -- name -> handle
        by_group = {}, -- group -> {handle -> true}
        defaults = {}, -- core entries, recorded by rune.registry._seal
        next_id = 1,
    }, Registry)
    registries[#registries + 1] = registry
    return registry
end

local function sort_list(list)
    table.sort(list, function(a, b)
        if a.priority ~= b.priority then
            return a.priority < b.priority
        end
        return a.id < b.id
    end)
end

-- A copy of `list` with `data` added in (priority, id) order.
local function sorted_with(list, data)
    local copy = {}
    for i, item in ipairs(list) do
        copy[i] = item
    end
    copy[#copy + 1] = data
    sort_list(copy)
    return copy
end

-- A copy of `list` without `data`.
local function without(list, data)
    local copy = {}
    for _, item in ipairs(list) do
        if item ~= data then
            copy[#copy + 1] = item
        end
    end
    return copy
end

local EMPTY = {}

-- Check opts before anything changes: a bad option must not cost the
-- entry it would have replaced. Unknown options are ignored with a notice.
local function check_options(registry, opts, api)
    if opts == nil then
        return {}
    end
    if type(opts) ~= "table" then
        rune._raise(api .. ": opts must be a table")
    end
    for key, want in pairs(COMMON_OPTIONS) do
        if opts[key] ~= nil and type(opts[key]) ~= want then
            rune._raise(api .. ": opts." .. key .. " must be a " .. want)
        end
    end
    for key in pairs(opts) do
        if not registry.allowed[key] then
            local source = rune.caller_source(3)
            rune.echo(rune.style.yellow("[Warning]") .. " " .. api .. ": unknown option '" ..
                tostring(key) .. "' ignored" .. (source and (" @" .. source) or ""))
        end
    end
    return opts
end

-- Put an entry (new, or a core default being restored) into the indexes.
function Registry:_insert(data)
    data.removed = nil
    local handle = data._handle
    if data.name and self.by_name[data.name] then
        self.by_name[data.name]:remove() -- upsert: the new entry replaces the old
    end
    self.entries = sorted_with(self.entries, data)
    if self.index_by then
        local value = data[self.index_by]
        self.indexed[value] = sorted_with(self.indexed[value] or EMPTY, data)
    end
    if data.name then
        self.by_name[data.name] = handle
    end
    if data.group then
        local members = self.by_group[data.group]
        if not members then
            members = {}
            self.by_group[data.group] = members
        end
        members[handle] = true
    end
    if self.on_add then
        self.on_add(data)
    end
    if self.on_change then
        self.on_change()
    end
end

-- Register an entry. opts: name, group, priority, once, plus spec.options.
-- `api` names the public function for messages (default "rune.<kind>").
-- The caller sets module fields, including `action` and `source` (only
-- the caller knows its stack depth for rune.caller_source).
function Registry:add(data, opts, api)
    opts = check_options(self, opts, api or ("rune." .. self.kind))
    data.id = self.next_id
    self.next_id = self.next_id + 1
    data.enabled = true
    data.priority = opts.priority or 50
    data.name = opts.name
    data.group = opts.group
    data.once = opts.once or false
    data._handle = setmetatable({ _data = data, _registry = self }, Handle)
    self:_insert(data)
    return data._handle
end

function Registry:_remove_data(data)
    if data.removed then
        return
    end
    data.removed = true -- tombstone: skipped by active() in snapshots
    self.entries = without(self.entries, data)
    if self.index_by then
        local value = data[self.index_by]
        local list = self.indexed[value]
        if list then
            list = without(list, data)
            self.indexed[value] = #list > 0 and list or nil
        end
    end
    if data.name and self.by_name[data.name] == data._handle then
        self.by_name[data.name] = nil
    end
    if data.group and self.by_group[data.group] then
        self.by_group[data.group][data._handle] = nil
    end
    if self.on_remove then
        self.on_remove(data)
    end
    if self.on_change then
        self.on_change()
    end
end

-- Run an entry's action protected, under the registry's label.
function Registry:call(data, ...)
    return rune.guarded_call(self.label, data, data.action, ...)
end

function Registry:get(name)
    return self.by_name[name]
end

-- The entry registered under `name`, or nil.
function Registry:entry(name)
    local handle = self.by_name[name]
    return handle and handle._data
end

local function by_name(verb)
    return function(self, name)
        local handle = self.by_name[name]
        if handle then
            handle[verb](handle)
            return true
        end
        return false
    end
end

Registry.enable = by_name("enable")
Registry.disable = by_name("disable")
Registry.toggle = by_name("toggle")
Registry.remove = by_name("remove")

-- The sorted entry list as of now. Dispatch loops iterate it directly:
-- changes replace the list, so a callback that changes the registry
-- cannot skip or double-run neighbors. Do not mutate it.
function Registry:snapshot()
    return self.entries
end

function Registry:count()
    return #self.entries
end

-- Remove user entries (those `match` accepts, or all), then restore every
-- matching core default: re-inserted if it was removed or replaced,
-- re-enabled, and its failure count reset.
function Registry:clear(match)
    local doomed = {}
    for _, data in ipairs(self.entries) do
        if not data.core and (not match or match(data)) then
            doomed[#doomed + 1] = data
        end
    end
    for _, data in ipairs(doomed) do
        self:_remove_data(data)
    end
    for _, data in ipairs(self.defaults) do
        if not match or match(data) then
            if data.removed then
                self:_insert(data)
            end
            data._handle:enable()
        end
    end
end

-- Remove all entries in a group; returns how many were removed.
function Registry:remove_group(group_name)
    if not group_name or not self.by_group[group_name] then
        return 0
    end
    local handles = {}
    for handle in pairs(self.by_group[group_name]) do
        handles[#handles + 1] = handle
    end
    for _, handle in ipairs(handles) do
        handle:remove()
    end
    return #handles
end

-- Listing rows: name, group, enabled, source, core, plus whatever
-- `extra(row, data)` adds. In registry order.
function Registry:list(extra)
    local rows = {}
    for _, data in ipairs(self.entries) do
        local row = {
            name = data.name,
            group = data.group,
            enabled = data.enabled,
            source = data.source,
            core = data.core or nil,
        }
        if extra then
            extra(row, data)
        end
        rows[#rows + 1] = row
    end
    return rows
end

local VERBS = { "get", "enable", "disable", "toggle", "remove", "count", "remove_group" }

-- Install the shared management functions on a public namespace.
-- clear() takes no arguments here; a module that filters (hooks by
-- event) defines its own.
function Registry:expose(ns)
    for _, verb in ipairs(VERBS) do
        ns[verb] = function(...)
            return self[verb](self, ...)
        end
    end
    ns.clear = function()
        self:clear()
    end
    return ns
end

-- INTERNAL: called once by the loader after the core has loaded. What
-- every registry holds now is Rune's defaults.
function rune.registry._seal()
    for _, registry in ipairs(registries) do
        registry.defaults = {}
        for i, data in ipairs(registry.entries) do
            data.core = true
            registry.defaults[i] = data
        end
    end
end

-- Group System (Control Only)
-- Manages the master enable/disable state for groups.
-- Item deletion is handled by each module (alias, trigger, timer, hooks).
--
-- Two-level enable/disable:
--   - Group level: master switch (rune.group.disable/enable)
--   - Item level: individual state (handle:disable/enable)
--
-- An item fires only if BOTH are enabled.
-- Group disable doesn't mutate individual states - they're preserved for re-enable.

rune.group = {}

-- Master switch state: group_name -> bool (nil = enabled)
local group_states = {}

-- Check if a group is enabled (used by alias/trigger/timer modules)
function rune.group.is_enabled(group_name)
    if not group_name then return true end
    if group_states[group_name] == false then
        return false
    end
    return true
end

-- Flip a group's master switch and notify the registries it covers.
local function set_group_enabled(group_name, enabled)
    if not group_name or rune.group.is_enabled(group_name) == enabled then
        return
    end
    group_states[group_name] = enabled
    for _, registry in ipairs(registries) do
        if registry.on_change and next(registry.by_group[group_name] or {}) then
            registry.on_change()
        end
    end
end

-- Disable a group (master switch off)
function rune.group.disable(group_name)
    set_group_enabled(group_name, false)
end

-- Enable a group (master switch on)
function rune.group.enable(group_name)
    set_group_enabled(group_name, true)
end

-- Flip a group's master switch
function rune.group.toggle(group_name)
    set_group_enabled(group_name, not rune.group.is_enabled(group_name))
end

-- List all known groups, aggregated from every registry.
-- Returns array of {name, enabled}.
function rune.group.list()
    local seen = {}

    for _, registry in ipairs(registries) do
        for group_name, members in pairs(registry.by_group) do
            if next(members) then
                seen[group_name] = true
            end
        end
    end

    -- Also include any explicitly toggled groups (even if empty)
    for group_name in pairs(group_states) do
        seen[group_name] = true
    end

    local result = {}
    for group_name in pairs(seen) do
        table.insert(result, {
            name = group_name,
            enabled = rune.group.is_enabled(group_name),
        })
    end
    table.sort(result, function(a, b) return a.name < b.name end)
    return result
end

-- An item fires only if it has not been removed, is individually
-- enabled, and its group's master switch is on.
function Registry:active(data)
    if data.removed or not data.enabled then
        return false
    end
    return rune.group.is_enabled(data.group)
end
