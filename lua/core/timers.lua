-- Timer System
-- Timers execute actions after a delay or repeatedly at intervals.
-- Built on rune.registry (registry.lua).
--
-- API:
--   rune.timer.after(seconds, action, opts?)  -- One-shot timer
--   rune.timer.every(seconds, action, opts?)  -- Repeating timer
--
-- every() uses fixed-interval scheduling: the next firing is
-- scheduled the moment the previous one fires, regardless of how
-- long the action takes to run.
--
-- Disabling suppresses firing. A repeating timer keeps its schedule
-- and resumes on re-enable; a one-shot whose moment passes while
-- disabled is removed - its wake-up is spent and cannot recur.
--
-- Returns a handle with :disable(), :enable(), :cancel(), :name(), :group()
-- and :remaining() (seconds until the next wake-up, or nil after removal).
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

local function remaining(self)
    local data = self._data
    if data.removed then
        return nil
    end
    return rune._timer.remaining(data.timer_id)
end

-- Create a timer (internal)
local function create_timer(seconds, action, opts, repeating)
    local api = repeating and "rune.timer.every" or "rune.timer.after"
    if type(seconds) ~= "number" or not (seconds > 0) or seconds == math.huge then
        rune._raise(api .. ": seconds must be a positive number")
    end
    if type(action) ~= "function" and type(action) ~= "string" then
        rune._raise(api .. ": action must be a function or string")
    end
    local data = {
        seconds = seconds,
        action = action,
        repeating = repeating,
        source = rune.caller_source(2),
    }

    local handle = registry:add(data, opts, api)
    handle.cancel = handle.remove -- :cancel() is intuitive for timers
    handle.remaining = remaining

    local function callback()
        -- Individual state AND group master switch
        if not registry:active(data) then
            -- The Go wake-up for a one-shot is spent: it can never
            -- fire again, so remove the entry rather than leaving a
            -- zombie in the registry that re-enabling cannot revive.
            if not data.repeating then
                handle:remove()
            end
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
            registry:call(data, ctx)
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
rune.timer = registry:expose({})
rune.timer.cancel = rune.timer.remove -- cancel is the natural verb for timers

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

-- List all timers. Remaining seconds come from the Go schedule, even when
-- callbacks are suppressed by disable/group state. A spent wake-up reads zero.
function rune.timer.list()
    return registry:list(function(row, data)
        row.seconds = data.seconds
        row.remaining = rune._timer.remaining(data.timer_id)
        row.mode = data.repeating and "every" or "after"
        row.value = type(data.action) == "function" and "(function)" or tostring(data.action)
    end)
end
