-- Shared Registry Factory
-- Hooks, timers, aliases, and triggers are all the same shape: a
-- registry of callbacks with handles, upsert-by-name, group
-- membership, priority ordering, and two-level enable/disable. This
-- factory implements that machinery exactly once, so every module has
-- identical semantics and a fix here fixes all of them.
--
-- Usage:
--   local reg = rune.registry.new{
--       kind = "trigger",               -- label for messages
--       on_add = function(data) end,    -- optional, after insertion
--       on_remove = function(data) end, -- optional, after removal
--   }
--   local handle = reg:add(data, opts)
--
-- The module owns `data` (pattern, action, ...); the factory
-- standardizes these fields on it:
--   id       -- insertion order (tiebreak for equal priorities)
--   enabled  -- individual switch (see reg:active)
--   priority -- opts.priority or 50, lower runs first
--   name     -- opts.name, unique: adding a duplicate replaces the old
--   group    -- opts.group, master-switch membership (25_groups.lua)
--   once     -- opts.once, module removes the item after first fire
--   _handle  -- back-reference to the handle
--
-- Handle API: :enable() :disable() :remove() :name() :group()

rune.registry = {}

local Handle = {}
Handle.__index = Handle

function Handle:enable()
    self._data.enabled = true
    return self
end

function Handle:disable()
    self._data.enabled = false
    return self
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

local Registry = {}
Registry.__index = Registry

function rune.registry.new(opts)
    opts = opts or {}
    return setmetatable({
        kind = opts.kind or "item",
        on_add = opts.on_add,
        on_remove = opts.on_remove,
        list = {},     -- all items, sorted by (priority, id)
        by_name = {},  -- name -> handle
        by_group = {}, -- group -> {handle -> true}
        next_id = 1,
    }, Registry)
end

local function sort_list(list)
    table.sort(list, function(a, b)
        if a.priority ~= b.priority then
            return a.priority < b.priority
        end
        return a.id < b.id
    end)
end

-- Register an item. opts: name, group, priority, once.
-- The caller sets module-specific fields (including `source`, since
-- only the caller knows its stack depth for rune.caller_source).
function Registry:add(data, opts)
    opts = opts or {}

    data.id = self.next_id
    self.next_id = self.next_id + 1
    data.enabled = true
    data.priority = opts.priority or 50
    data.name = opts.name
    data.group = opts.group
    data.once = opts.once or false

    local handle = setmetatable({
        _data = data,
        _registry = self,
    }, Handle)
    data._handle = handle

    -- Upsert: a new item with an existing name replaces the old one
    if data.name and self.by_name[data.name] then
        self.by_name[data.name]:remove()
    end

    table.insert(self.list, data)
    sort_list(self.list)

    if data.name then
        self.by_name[data.name] = handle
    end
    if data.group then
        local grp = self.by_group[data.group]
        if not grp then
            grp = {}
            self.by_group[data.group] = grp
        end
        grp[handle] = true
    end

    if self.on_add then
        self.on_add(data)
    end

    return handle
end

function Registry:_remove_data(data)
    for i, item in ipairs(self.list) do
        if item == data then
            table.remove(self.list, i)
            break
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
end

function Registry:get(name)
    return self.by_name[name]
end

function Registry:enable(name)
    local handle = self.by_name[name]
    if handle then
        handle:enable()
        return true
    end
    return false
end

function Registry:disable(name)
    local handle = self.by_name[name]
    if handle then
        handle:disable()
        return true
    end
    return false
end

function Registry:remove(name)
    local handle = self.by_name[name]
    if handle then
        handle:remove()
        return true
    end
    return false
end

-- The sorted item list, for dispatch iteration. Do not mutate;
-- removal during iteration must go through handles after the loop.
function Registry:items()
    return self.list
end

function Registry:count()
    return #self.list
end

-- Remove every item (on_remove fires for each).
function Registry:clear()
    local handles = {}
    for _, data in ipairs(self.list) do
        handles[#handles + 1] = data._handle
    end
    for _, handle in ipairs(handles) do
        handle:remove()
    end
end

-- Remove all items in a group; returns how many were removed.
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

-- An item fires only if individually enabled AND its group's master
-- switch is on. rune.group loads after this file but exists by the
-- time anything dispatches.
function Registry:active(data)
    if not data.enabled then
        return false
    end
    return not rune.group or rune.group.is_enabled(data.group)
end
