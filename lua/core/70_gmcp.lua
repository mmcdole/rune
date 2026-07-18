-- GMCP (Generic MUD Communication Protocol)
-- Go owns the transport (option 201 framing, JSON encode/decode via
-- the shared bridge); this module owns the policy: the Core.Hello
-- handshake, package subscriptions, and handler dispatch.
--
-- Handlers are registry-based, so they get names, groups, priorities,
-- failure quarantine, and source attribution like every other
-- callback. Package names match case-insensitively (per the spec).

local green, red, yellow, dim =
    rune.style.green, rune.style.red, rune.style.yellow, rune.style.gray

rune.gmcp = {}

local subscriptions = {} -- package -> version (what Core.Supports.Set sends)

-- Core.Supports.Set replaces the server-side set wholesale, so send
-- the full list every time.
local function send_supports()
    local list = {}
    for package, version in pairs(subscriptions) do
        table.insert(list, package .. " " .. version)
    end
    table.sort(list)
    if #list > 0 then
        rune._gmcp.send("Core.Supports.Set", list)
    end
end

-- Per-package dispatch index (mirrors 20_hooks.lua's by_event)
local by_package = {} -- lowercase package -> sorted array of data

local function sort_handlers(handlers)
    table.sort(handlers, function(a, b)
        if a.priority ~= b.priority then
            return a.priority < b.priority
        end
        return a.id < b.id
    end)
end

local registry = rune.registry.new{
    kind = "gmcp",
    on_add = function(data)
        local handlers = by_package[data.package]
        if not handlers then
            handlers = {}
            by_package[data.package] = handlers
        end
        table.insert(handlers, data)
        sort_handlers(handlers)
    end,
    on_remove = function(data)
        local handlers = by_package[data.package]
        if not handlers then
            return
        end
        for i, entry in ipairs(handlers) do
            if entry == data then
                table.remove(handlers, i)
                break
            end
        end
    end,
}

-- Attach a handler to a GMCP package (exact match, case-insensitive).
-- handler receives (data, package): data is the decoded JSON value
-- (table/string/number/boolean, or nil when the message had no body),
-- package is the name as the server sent it.
-- opts: name, group, priority (see 15_registry.lua).
-- Returns a handle with :enable/:disable/:remove.
function rune.gmcp.on(package, handler, opts)
    return registry:add({
        package = package:lower(),
        handler = handler,
        source = rune.caller_source(1),
    }, opts)
end

function rune.gmcp.remove(name)
    return registry:remove(name)
end

function rune.gmcp.enable(name)
    return registry:enable(name)
end

function rune.gmcp.disable(name)
    return registry:disable(name)
end

-- Send a GMCP message. data may be a string, number, boolean, or
-- JSON-able table; nil sends the bare package name.
-- Echoes failures (not connected, GMCP not negotiated) rather than
-- raising. Returns true, or nil + error message.
function rune.gmcp.send(package, data)
    local ok, err = rune._gmcp.send(package, data)
    if not ok then
        rune.echo(red("[GMCP]") .. " " .. tostring(err))
    end
    return ok, err
end

-- Send with a pre-encoded JSON string (no validation). Debug tool.
function rune.gmcp.send_raw(package, raw)
    return rune._gmcp.send_raw(package, raw)
end

-- True while GMCP is negotiated on the current connection. Queried
-- live from Go rather than cached here: negotiation is connection-
-- lifetime state, and a VM-lifetime copy would go stale across
-- /reload (the gmcp_enabled edge fires once per connection).
function rune.gmcp.is_enabled()
    return rune._gmcp.is_active()
end

-- Subscribe to server packages ("Char", "Room", ...). Takes effect
-- immediately when GMCP is up, otherwise at the next handshake.
-- version defaults to 1.
function rune.gmcp.subscribe(package, version)
    subscriptions[package] = version or 1
    if rune._gmcp.is_active() then
        send_supports()
    end
end

function rune.gmcp.unsubscribe(package)
    if subscriptions[package] == nil then
        return
    end
    subscriptions[package] = nil
    if rune._gmcp.is_active() then
        send_supports()
    end
end

-- List handlers for /gmcp: array of {package, name, enabled, group, source}.
function rune.gmcp.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
        table.insert(result, {
            package = data.package,
            name = data.name,
            enabled = data.enabled,
            group = data.group,
            source = data.source,
        })
    end
    return result
end

-- INTERNAL: called by Go (Engine.OnGMCP) with the package name, the
-- decoded value, and the raw JSON text. Runs the catch-all "gmcp"
-- hook first, then the package-specific handlers.
function rune.gmcp._dispatch(package, data, raw)
    rune.hooks.call("gmcp", package, data, raw)

    local live = by_package[package:lower()]
    if not live or #live == 0 then
        return
    end
    -- Snapshot: a handler may add/remove handlers mid-dispatch.
    local handlers = {}
    for i, entry in ipairs(live) do
        handlers[i] = entry
    end
    for _, entry in ipairs(handlers) do
        if registry:active(entry) then
            local label = 'GMCP "' .. package .. '"' ..
                (entry.name and (' "' .. entry.name .. '"') or "") ..
                (entry.source and (" @" .. entry.source) or "")
            rune.guarded_call(label, entry, entry.handler, data, package)
        end
    end
end

-- Handshake policy: when the server negotiates GMCP, introduce
-- ourselves and declare subscriptions. Visible and overridable - hide
-- or replace it like any named hook.
rune.hooks.on("gmcp_enabled", function()
    rune._gmcp.send("Core.Hello", { client = "Rune", version = rune.version })
    send_supports()
end, { name = "gmcp-hello", priority = 100 })

-- /gmcp - status, or send a raw message for debugging
rune.command.add("gmcp", function(args)
    local sub, rest = args:match("^(%S*)%s*(.*)$")
    if sub == "send" then
        local package, raw = rest:match("^(%S+)%s*(.*)$")
        if not package then
            rune.echo("[Usage] /gmcp send <package> [json]")
            return
        end
        local ok, err = rune.gmcp.send_raw(package, raw ~= "" and raw or nil)
        if ok then
            rune.echo(green("[GMCP]") .. " sent " .. package)
        else
            rune.echo(red("[GMCP]") .. " " .. tostring(err))
        end
        return
    end

    local status = rune._gmcp.is_active() and green("negotiated") or dim("not negotiated")
    rune.echo(green("[GMCP]") .. " " .. status)
    local subs = {}
    for package, version in pairs(subscriptions) do
        table.insert(subs, package .. " " .. version)
    end
    table.sort(subs)
    rune.echo("  subscriptions: " ..
        (#subs > 0 and table.concat(subs, ", ") or dim("(none)")))
    local handlers = rune.gmcp.list()
    rune.echo("  handlers: " .. dim("(" .. #handlers .. " total)"))
    for _, h in ipairs(handlers) do
        local status_str = h.enabled and green("[on] ") or red("[off]")
        local name_str = h.name and ("  " .. dim("name:") .. h.name) or ""
        local src_str = h.source and ("  " .. dim("@" .. h.source)) or ""
        rune.echo(string.format("  %s %-24s%s%s",
            status_str, yellow(h.package), name_str, src_str))
    end
end, "GMCP status and debugging (/gmcp, /gmcp send <package> [json])")
