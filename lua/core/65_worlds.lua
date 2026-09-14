-- World Bookmarks
-- Named servers, stored durably in rune.store under the "worlds" key
-- (so they live in <config>/store.json and survive restarts).
-- /connect resolves world names before host:port parsing, and with no
-- arguments opens a picker over the saved worlds (see 55_commands.lua).

local green, red, yellow, dim =
    rune.style.green, rune.style.red, rune.style.yellow, rune.style.gray

rune.world = {}

local KEY = "worlds"

local function load_worlds()
    return rune.store.get(KEY) or {}
end

-- Add or replace a world. Extra opts keys are stored verbatim
-- alongside the address (room for future fields like on_connect).
-- Returns true, or nil + error message.
function rune.world.add(name, address, opts)
    if type(name) ~= "string" or name == "" or name:find("[%s:/]") then
        return nil, "world names cannot contain spaces, ':' or '/'"
    end
    if type(address) ~= "string" or not address:find(":") then
        return nil, "address must be host:port (optionally tls:// or tls+insecure://)"
    end
    local entry = {}
    if type(opts) == "table" then
        for k, v in pairs(opts) do entry[k] = v end
    end
    entry.address = address
    local worlds = load_worlds()
    worlds[name] = entry
    return rune.store.set(KEY, worlds)
end

-- Remove a world. Returns true if it existed.
function rune.world.remove(name)
    local worlds = load_worlds()
    if not worlds[name] then
        return false
    end
    worlds[name] = nil
    rune.store.set(KEY, worlds)
    return true
end

-- Returns the stored entry table ({address=..., ...}), or nil.
function rune.world.get(name)
    return load_worlds()[name]
end

-- Returns a sorted array of {name, address}.
function rune.world.list()
    local result = {}
    for name, entry in pairs(load_worlds()) do
        table.insert(result, { name = name, address = entry.address })
    end
    table.sort(result, function(a, b) return a.name < b.name end)
    return result
end

local function print_worlds()
    local worlds = rune.world.list()
    rune.echo(green("[Worlds]") .. dim(" (" .. #worlds .. " total)"))
    if #worlds == 0 then
        rune.echo("  " .. dim("(none - /world add <name> <host> <port> [tls])"))
        return
    end
    local last = rune.store.get("last_address")
    for _, w in ipairs(worlds) do
        local marker = (w.address == last) and green(" *") or "  "
        rune.echo(string.format(" %s %-16s %s", marker, yellow(w.name), w.address))
    end
end

-- Parse "<host> <port> [tls|tls+insecure]" or a bare address into an
-- address string. Returns nil on bad scheme/shape.
local function parse_address(text)
    local host, port, scheme = text:match("^(%S+)%s+(%d+)%s*(%S*)$")
    if host then
        if scheme == "" then
            return host .. ":" .. port
        elseif scheme == "tls" or scheme == "tls+insecure" then
            return scheme .. "://" .. host .. ":" .. port
        end
        return nil
    end
    local addr = text:match("^(%S+)$")
    if addr and addr:find(":") then
        return addr
    end
    return nil
end

rune.command.add("world", function(args)
    local sub, rest = args:match("^(%S*)%s*(.*)$")
    if sub == "" or sub == "list" then
        print_worlds()
    elseif sub == "add" then
        local name, addr_text = rest:match("^(%S+)%s+(.+)$")
        local address = addr_text and parse_address(addr_text)
        if not address then
            rune.echo("[Usage] /world add <name> <host> <port> [tls|tls+insecure]")
            return
        end
        local ok, err = rune.world.add(name, address)
        if ok then
            rune.echo(green("[World]") .. " " .. name .. " -> " .. address)
        else
            rune.echo(red("[Error]") .. " " .. tostring(err))
        end
    elseif sub == "remove" then
        local name = rest:match("^(%S+)$")
        if not name then
            rune.echo("[Usage] /world remove <name>")
            return
        end
        if rune.world.remove(name) then
            rune.echo(green("[World]") .. " removed " .. name)
        else
            rune.echo(red("[Error]") .. " no world named " .. name)
        end
    else
        rune.echo("[Usage] /world [list] | /world add <name> <host> <port> [tls|tls+insecure] | /world remove <name>")
    end
end, "Manage world bookmarks (/world add|remove|list)")

-- /worlds - shorthand for /world list
rune.command.add("worlds", function(args)
    print_worlds()
end, "List saved worlds")
