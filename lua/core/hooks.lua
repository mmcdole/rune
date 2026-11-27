-- Hooks
-- Event handlers: inspect or change input, output, prompts, and echo, and
-- react to client events. Several handlers per event, in priority order.
--
-- Go fires the events (Engine.callHooks / notify); this file owns the
-- handler registry and how each event chains its handlers' results.
--
-- API:
--   rune.hooks.on(event, handler, opts?)   -- returns a handle
--   rune.hooks.call(event, ...)            -- internal: run an event's chain
--   rune.hooks.has(event)  rune.hooks.count(event?)  rune.hooks.clear(event?)
--   rune.hooks.list()                      -- {event, priority, name, ...}
--   + shared management (get, enable, disable, toggle, remove,
--     remove_group); see registry.lua
--
-- Options (beyond name, group):
--   priority = 50   -- lower runs first
--
-- Notes:
--   Data-flow events chain results:
--     "input"   (text, context) -- a string rewrites, false consumes.
--                context.mode is "command" or "verbatim"; context.hidden
--                is true while the server hides input (a password prompt)
--     "output"  (line)          -- false gags, a string rewrites
--     "prompt"  (line, confirmed) -- confirmed is false for a partial
--                line, true for a GA/EOR-confirmed prompt
--     "echo"    (text)          -- false hides, a string rewrites; the
--                core echo-style handler adds the "> "
--   Notification events ignore return values: ready, connecting,
--   connected, disconnecting, disconnected, reloading, reloaded, loaded,
--   error, input_changed, window_size_changed, gmcp (package, data, raw;
--   a catch-all beside rune.gmcp.on), gmcp_enabled.

local green, red, cyan, gray = rune.style.green, rune.style.red, rune.style.cyan, rune.style.gray

-- A failing handler is reported under this label. Built only on failure.
local function hook_label(entry)
    return "Hook " .. entry.event .. " " ..
        (entry.name and ('"' .. entry.name .. '"') or ("#" .. entry.id)) ..
        (entry.source and (" @" .. entry.source) or "")
end

local registry = rune.registry.new{
    kind = "hook",
    index_by = "event",
    label = hook_label,
}

-- Per-event dispatch lists (event -> sorted entries), so rune.hooks.call
-- doesn't scan unrelated events on every line.
local by_event = registry.indexed
local no_handlers = {}

-- Public API

rune.hooks = registry:expose({})

-- Attach a handler to an event. Returns its handle.
function rune.hooks.on(event, handler, opts)
    if type(event) ~= "string" then
        error("rune.hooks.on: event must be a string", 2)
    end
    if type(handler) ~= "function" then
        error("rune.hooks.on: handler must be a function", 2)
    end
    return registry:add({
        event = event,
        action = handler,
        source = rune.caller_source(1),
    }, opts, "rune.hooks.on")
end

-- List all registered handlers
function rune.hooks.list()
    return registry:list(function(row, entry)
        row.event = entry.event
        row.priority = entry.priority
    end)
end

-- Remove user handlers for an event (or all events) and restore Rune's
-- own handlers there.
function rune.hooks.clear(event)
    if event == nil then
        return registry:clear()
    end
    registry:clear(function(entry)
        return entry.event == event
    end)
end

-- Check if any handlers are registered for an event
function rune.hooks.has(event)
    return by_event[event] ~= nil
end

-- Count handlers for an event, or all handlers
function rune.hooks.count(event)
    if event then
        return by_event[event] and #by_event[event] or 0
    end
    return registry:count()
end

-- Dispatch

-- Run a single handler protected. A failing handler is reported and
-- treated as returning nil (pass through), so one broken handler
-- cannot abort the rest of the chain or kill trigger processing.
-- Errors are echoed directly (not re-dispatched through the "error"
-- event) to avoid recursion when an error handler itself fails.
-- Repeated failures disable the handler (see rune.guarded_call).
local function run_handler(entry, ...)
    -- guarded_call directly, not registry:call: this runs for every
    -- handler on every line, and the extra frame is measurable.
    local ok, result = rune.guarded_call(hook_label, entry, entry.action, ...)
    if ok then
        return result
    end
    return nil
end

-- Give each input handler its own read-only context.
local function reject_input_context_write()
    error("rune.hooks: input context is read-only", 2)
end

-- One shared metatable per (mode, hidden) pair.
local input_context_metatables = {}
for _, mode in ipairs({ "command", "verbatim" }) do
    input_context_metatables[mode] = {}
    for _, hidden in ipairs({ true, false }) do
        input_context_metatables[mode][hidden] = {
            __index = { mode = mode, hidden = hidden },
            __newindex = reject_input_context_write,
            __metatable = false,
        }
    end
end

local function input_context(mode, hidden)
    local by_hidden = input_context_metatables[mode] or input_context_metatables.command
    return setmetatable({}, by_hidden[hidden == true])
end

-- Output and prompt chain Line objects; strings replace the object seen by
-- later handlers, and false gags it. Only prompt receives a second argument.
local function line_chain(event, handlers, line, confirmed)
    if event == "prompt" and #handlers > 0 and type(confirmed) ~= "boolean" then
        error("rune.hooks.call: prompt requires an explicit confirmed boolean", 2)
    end
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            local result
            if event == "prompt" then
                result = run_handler(entry, line, confirmed)
            else
                result = run_handler(entry, line)
            end
            if result == false then
                return "", false
            elseif type(result) == "string" then
                line = rune.line.new(result)
            end
        end
    end
    return line:raw(), true
end

-- Echo chains display text; false hides the local echo.
local function text_chain(_, handlers, text)
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            local result = run_handler(entry, text)
            if result == false then
                return "", false
            elseif type(result) == "string" then
                text = result
            end
        end
    end
    return text, true
end

-- Input chains physical lines. Reject newlines before subsequent handlers
-- can act on them, and isolate each handler's view of the canonical mode.
local function run_input_hooks(_, handlers, text, context)
    local mode = context and context.mode or "command"
    local hidden = context and context.hidden
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            local result = run_handler(entry, text, input_context(mode, hidden))
            if result == false then
                return false
            elseif type(result) == "string" then
                if result:find("[\r\n]") then
                    error("input rewrite must stay on one line", 0)
                end
                text = result
            end
        end
    end
    return text
end

-- Notifications preserve the full argument list, including embedded/trailing
-- nil values. Handler return values do not affect subsequent notifications.
local function notify(_, handlers, ...)
    local nargs = select("#", ...)
    local args = {...}
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            run_handler(entry, unpack(args, 1, nargs))
        end
    end
end

local chains = {
    output = line_chain,
    prompt = line_chain,
    echo = text_chain,
    input = run_input_hooks,
}

-- Membership is fixed for the call: registrations replace by_event's array
-- rather than mutating it. Additions wait for the next dispatch; removals
-- are honored by each chain's registry:active check.
function rune.hooks.call(event, ...)
    return (chains[event] or notify)(event, by_event[event] or no_handlers, ...)
end

-- Defaults

-- /hooks - List all hooks
rune.command.add("hooks", function()
    local hooks = rune.hooks.list()
    rune.echo(green("[Hooks]") .. gray(" (" .. #hooks .. " total)"))
    if #hooks == 0 then
        rune.echo("  " .. gray("(none)"))
        return
    end
    for _, h in ipairs(hooks) do
        local status = h.enabled and green("[on] ") or red("[off]")
        local group_str = h.group and ("  " .. cyan("<" .. h.group .. ">")) or ""
        local name_str = h.name or "(anonymous)"
        local pri_str = gray("pri:") .. tostring(h.priority)
        local src_str = rune.registry._origin(h)
        rune.echo(string.format("  %s %-12s %s %s %s%s%s",
            status, h.event, pri_str, gray("->"), name_str, group_str, src_str))
    end
end, "List all hooks")
