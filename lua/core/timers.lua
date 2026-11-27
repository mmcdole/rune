-- Timers
-- Run an action after a delay, or repeatedly at an interval.
--
-- Go schedules the wake-ups (rune._timer); this file owns the registry,
-- the actions, and what disabling means.
--
-- API:
--   rune.timer.after(seconds, action, opts?)  -- one-shot
--   rune.timer.every(seconds, action, opts?)  -- repeating
--   rune.timer.list()                         -- {seconds, remaining, mode, ...}
--   + shared management (get, enable, disable, toggle, remove, count,
--     clear, remove_group); see registry.lua. cancel is remove.
--
-- Notes:
--   An action is a string (sent as a command) or function(ctx), with
--   ctx = {name, group, type = "timer", remove()}. Handles add
--   :remaining(), seconds until the next wake-up (nil once removed).
--
--   every() uses fixed-interval scheduling: the next firing is scheduled
--   when the previous one fires, however long the action runs. Disabling
--   suppresses firing; a repeating timer keeps its schedule, and a
--   one-shot whose moment passes while disabled is removed.

local green, red, cyan, gray = rune.style.green, rune.style.red, rune.style.cyan, rune.style.gray

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

-- Dispatch

-- INTERNAL: called by Go when a timer fires. Unknown ids - cancelled
-- mid-flight, or scheduled by a previous VM generation - are ignored.
function rune.timer._fire(id)
    local callback = pending[id]
    if callback then
        callback()
    end
end

-- Defaults

-- /timers - List all timers
rune.command.add("timers", function()
    local timers = rune.timer.list()
    rune.echo(green("[Timers]") .. gray(" (" .. #timers .. " total)"))
    if #timers == 0 then
        rune.echo("  " .. gray("(none)"))
        return
    end
    for _, t in ipairs(timers) do
        local status = t.enabled and green("[on] ") or red("[off]")
        local group_str = t.group and ("  " .. cyan("<" .. t.group .. ">")) or ""
        local name_str = t.name and (" " .. gray("name:") .. t.name) or ""
        local src_str = rune.registry._origin(t)
        local timing = string.format("%s %.1fs (%.1fs left)", t.mode, t.seconds, t.remaining)
        rune.echo(string.format("  %s %-12s %s %s%s%s%s",
            status, timing, gray("->"), t.value, group_str, name_str, src_str))
    end
end, "List all timers")
