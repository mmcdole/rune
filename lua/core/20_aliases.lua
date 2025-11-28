-- Alias System

rune.alias = {}
local storage = {}  -- Private storage

function rune.alias.add(key, value)
    storage[key] = value
    if type(value) == "function" then
        rune.dbg("alias.add: " .. key .. " -> (function)")
    else
        rune.dbg("alias.add: " .. key .. " -> " .. tostring(value))
    end
end

function rune.alias.remove(key)
    if storage[key] then
        storage[key] = nil
        rune.dbg("alias.remove: " .. key)
    end
end

function rune.alias.list()
    rune.print("[Aliases]")
    -- Collect and sort keys
    local keys = {}
    for k in pairs(storage) do
        table.insert(keys, k)
    end
    table.sort(keys)

    if #keys == 0 then
        rune.print("  (none)")
        return
    end

    for _, k in ipairs(keys) do
        local v = storage[k]
        if type(v) == "function" then
            rune.print("  " .. k .. " -> (function)")
        else
            rune.print("  " .. k .. " -> " .. v)
        end
    end
end

-- Internal: Get alias (used by command processor)
function rune.alias.get(key)
    return storage[key]
end

-- Clear all aliases
function rune.alias.clear()
    storage = {}
end

-- Count aliases
function rune.alias.count()
    local count = 0
    for _ in pairs(storage) do
        count = count + 1
    end
    return count
end

-- Run an alias programmatically (for calling aliases from other aliases)
function rune.alias.run(name, args)
    local value = storage[name]
    if not value then return false end

    if type(value) == "function" then
        value(args or "")
    else
        -- String alias - process through command system
        local expansion = value
        if args and args ~= "" then
            expansion = expansion .. " " .. args
        end
        rune.send(expansion)
    end
    return true
end
