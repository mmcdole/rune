-- Guard
-- How every user callback runs: protected, attributed, and quarantined
-- when it keeps failing. The Lua side of Go's Engine.guard.
--
-- Go owns the watchdog deadline and calls rune._quarantine_runaway after
-- interrupting a runaway entry; this file owns failure counting, the
-- notices, and source attribution.
--
-- API (internal):
--   rune.guarded_call(label, data, fn, ...)  -- run fn protected; ok, result
--   rune.caller_source(level)                -- "file:line" of a script frame
--   rune._raise(message)                     -- error at the user's frame
--
-- Notes:
--   Three consecutive failures disable an entry; one watchdog interruption
--   disables it at once. Enabling the entry again resets the count.

local MAX_CONSECUTIVE_FAILURES = 3

-- Entries currently inside guarded_call, outermost first. Lunar raises on
-- every instruction past the watchdog deadline, so an interrupted call never
-- pops its entry; rune._quarantine_runaway reads and clears what is left.
local active_labels = {}
local active_entries = {}

-- A label is a string or a function of the entry, so a hot dispatch path
-- builds its label only when something fails.
local function label_text(label, data)
    if type(label) == "function" then
        return label(data)
    end
    return label
end

local function disable_runaway(label, data)
    if data and data.enabled then
        data.enabled = false
        rune.echo(rune.style.yellow("[" .. label_text(label, data) .. "]") ..
            " disabled after exceeding the script watchdog")
    end
end

-- Run fn protected, tracking consecutive failures on `data` (any registry
-- entry). Errors are echoed under `label`; at the failure limit the entry
-- is disabled with a notice. A watchdog interruption disables it at once:
-- each repeat would stall the client for the full timeout. Returns ok and
-- fn's first result.
function rune.guarded_call(label, data, fn, ...)
    local depth = #active_entries + 1
    active_labels[depth] = label
    active_entries[depth] = data
    local ok, result = pcall(fn, ...)
    active_labels[depth] = nil
    active_entries[depth] = nil
    if ok then
        data.failures = nil
        return true, result
    end

    data.failures = (data.failures or 0) + 1
    label = label_text(label, data)
    rune.echo(rune.style.red("[" .. label .. "]") .. " error: " .. tostring(result))
    if rune._watchdog_expired() then
        disable_runaway(label, data)
    elseif data.failures >= MAX_CONSECUTIVE_FAILURES and data.enabled then
        data.enabled = false
        rune.echo(rune.style.yellow("[" .. label .. "]") .. " disabled after " ..
            data.failures .. " consecutive errors")
    end
    return false, nil
end

-- INTERNAL: called by Go under a fresh deadline after the watchdog
-- interrupts a Lua entry. The interrupted guarded_call never reached its
-- own check; the innermost active entry is the callback that was running.
function rune._quarantine_runaway()
    local depth = #active_entries
    local label, data = active_labels[depth], active_entries[depth]
    active_labels, active_entries = {}, {}
    disable_runaway(label, data)
end

-- Source attribution
-- Returns "file:line" of the script frame `level` levels above the caller
-- (1 = the caller's caller), or nil. Registries record it so listings and
-- errors can say whose code is involved.
function rune.caller_source(level)
    local getinfo = debug and debug.getinfo
    if not getinfo then
        return nil
    end

    local function format_source(info)
        local src = info.source:gsub("^@", "")
        if info.currentline and info.currentline > 0 then
            return src .. ":" .. info.currentline
        end
        return src
    end

    local function usable(info)
        return info ~= nil and info.source ~= nil
            and info.source ~= "=[C]" and info.source ~= "=(tail call)"
    end

    -- +2 skips this function and its caller. Engines differ on tail calls:
    -- a `return helper(...)` frame may be eliminated or left as a
    -- "(tail call)" pseudo frame. Probe upward past pseudo frames, then
    -- back down if the level overshot a shortened stack.
    local start = level + 2
    for l = start, start + 4 do
        local info = getinfo(l, "Sl")
        if not info then
            break
        end
        if usable(info) then
            return format_source(info)
        end
    end
    for l = start - 1, 1, -1 do
        local info = getinfo(l, "Sl")
        if usable(info) then
            return format_source(info)
        end
    end
    return nil
end

-- Raise `message` at the first stack frame outside the embedded core, so
-- argument checks inside shared helpers still point at the user's call.
-- Falls back to the immediate caller when every frame is core code.
function rune._raise(message)
    local getinfo = debug and debug.getinfo
    if getinfo then
        local level = 2
        while true do
            local info = getinfo(level, "S")
            if not info then
                break
            end
            local src = info.source or ""
            if src ~= "=[C]" and src ~= "=(tail call)" and not src:find("^@?core/") then
                error(message, level)
            end
            level = level + 1
        end
    end
    error(message, 2)
end
