-- Timer System
-- Timers execute actions after a delay or repeatedly at intervals.
-- Built on rune.registry (06_registry.lua).
--
-- API:
--   rune.timer.after(seconds, action, opts?)  -- One-shot timer
--   rune.timer.every(seconds, action, opts?)  -- Repeating timer
--
-- every() uses fixed-interval scheduling: the next firing is
-- scheduled the moment the previous one fires, regardless of how
-- long the action takes to run.
--
-- Returns a handle with :disable(), :enable(), :cancel(), :name(), :group()
--
-- Options:
--   name  = "string"  -- Unique ID for upsert/management
--   group = "string"  -- Group membership for bulk operations
--
-- Action can be:
--   - String: sent as command
--   - Function: function(ctx)
--       ctx = {name, group, type, remove()}

-- Go timer id -> callback. Go only schedules wake-ups; this module
-- owns dispatch, so all callback state dies with the VM on reload.
local pending = {}

local registry = rune.registry.new{
    kind = "timer",
    on_remove = function(data)
        -- Stop the underlying Go timer when the entry goes away
        if data.timer_id then
            rune._timer.cancel(data.timer_id)
            pending[data.timer_id] = nil
        end
    end,
}

-- Create a timer (internal)
local function create_timer(seconds, action, opts, repeating)
    local data = {
        seconds = seconds,
        action = action,
        repeating = repeating,
        source = rune.caller_source(2),
    }

    local handle = registry:add(data, opts)
    handle.cancel = handle.remove -- :cancel() is intuitive for timers

    local function callback()
        -- Individual state AND group master switch
        if not registry:active(data) then
            return
        end

        local ctx = {
            name = data.name,
            group = data.group,
            type = "timer",
        }
        function ctx:remove()
            handle:remove()
        end

        if type(data.action) == "function" then
            local label = (data.name and ('Timer "' .. data.name .. '"') or "Timer") ..
                (data.source and (" @" .. data.source) or "")
            rune.guarded_call(label, data, data.action, ctx)
        elseif type(data.action) == "string" and data.action ~= "" then
            rune.send(data.action)
        end

        -- Auto-remove one-shot timers after firing
        if not data.repeating then
            handle:remove()
        end
    end

    if repeating then
        data.timer_id = rune._timer.every(seconds)
    else
        data.timer_id = rune._timer.after(seconds)
    end
    pending[data.timer_id] = callback

    return handle
end

-- Public API
rune.timer = {}

-- INTERNAL: called by Go when a timer fires. Unknown ids - cancelled
-- mid-flight, or scheduled by a previous VM generation - are ignored.
function rune.timer._fire(id)
    local callback = pending[id]
    if callback then
        callback()
    end
end

-- One-shot timer (fires once after delay)
function rune.timer.after(seconds, action, opts)
    return create_timer(seconds, action, opts, false)
end

-- Repeating timer (fires every interval)
function rune.timer.every(seconds, action, opts)
    return create_timer(seconds, action, opts, true)
end

-- Management by name
function rune.timer.disable(name)
    return registry:disable(name)
end

function rune.timer.enable(name)
    return registry:enable(name)
end

function rune.timer.remove(name)
    return registry:remove(name)
end

-- Alias: cancel is intuitive for timers
rune.timer.cancel = rune.timer.remove

-- List all timers - returns array of {seconds, mode, value, name, enabled, group}
function rune.timer.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
        table.insert(result, {
            seconds = data.seconds,
            mode = data.repeating and "every" or "after",
            value = type(data.action) == "function" and "(function)" or tostring(data.action),
            name = data.name,
            enabled = data.enabled,
            group = data.group,
            source = data.source,
        })
    end
    return result
end

-- Clear all timers
function rune.timer.clear()
    registry:clear()
end

-- Count timers
function rune.timer.count()
    return registry:count()
end

-- Group operations
function rune.timer.remove_group(group_name)
    return registry:remove_group(group_name)
end
