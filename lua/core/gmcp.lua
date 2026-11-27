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

local registry = rune.registry.new{
    kind = "gmcp",
    index_by = "package",
    label = function(data)
        return 'GMCP "' .. data.package .. '"' ..
            (data.name and (' "' .. data.name .. '"') or "") ..
            (data.source and (" @" .. data.source) or "")
    end,
}

-- Per-package dispatch lists (lowercase package -> sorted entries).
local by_package = registry.indexed

-- Attach a handler to a GMCP package (exact match, case-insensitive).
-- handler receives (data, package): data is the decoded JSON value
-- (table/string/number/boolean, or nil when the message had no body),
-- package is the name as the server sent it.
-- opts: name, group, priority (see registry.lua).
-- Returns a handle with :enable/:disable/:remove.
function rune.gmcp.on(package, handler, opts)
    if type(package) ~= "string" then
        error("rune.gmcp.on: package must be a string", 2)
    end
    if type(handler) ~= "function" then
        error("rune.gmcp.on: handler must be a function", 2)
    end
    return registry:add({
        package = package:lower(),
        action = handler,
        source = rune.caller_source(1),
    }, opts, "rune.gmcp.on")
end

registry:expose(rune.gmcp)

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
    return registry:list(function(row, data)
        row.package = data.package
    end)
end

-- INTERNAL: called by Go (Engine.OnGMCP) with the package name, the
-- decoded value, and the raw JSON text. Runs the catch-all "gmcp"
-- hook first, then the package-specific handlers.
function rune.gmcp._dispatch(package, data, raw)
    rune.hooks.call("gmcp", package, data, raw)

    -- The list is replaced, never mutated, when handlers change, so
    -- handlers added or removed mid-dispatch can't perturb this loop.
    for _, entry in ipairs(by_package[package:lower()] or {}) do
        if registry:active(entry) then
            registry:call(entry, data, package)
        end
    end
end

-- Handshake: when the server negotiates GMCP, introduce ourselves.
-- Re-register "gmcp-hello" to send a different hello.
rune.hooks.on("gmcp_enabled", function()
    rune._gmcp.send("Core.Hello", { client = "Rune", version = rune.version })
end, { name = "gmcp-hello", priority = 100 })

-- Subscriptions belong to the subscription manager, not the hello: they
-- go out after it (a same-name hello replacement defaults to priority
-- 50), so replacing the hello can't drop them.
rune.hooks.on("gmcp_enabled", send_supports, { name = "_gmcp-supports", priority = 110 })

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
        local src_str = h.core and ("  " .. dim("core")) or (h.source and ("  " .. dim("@" .. h.source)) or "")
        rune.echo(string.format("  %s %-24s%s%s",
            status_str, yellow(h.package), name_str, src_str))
    end
end, "GMCP status and debugging (/gmcp, /gmcp send <package> [json])")
