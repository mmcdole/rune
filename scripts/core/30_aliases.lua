-- Alias System

rune.alias = {}
local storage = {}  -- Private storage

function rune.alias.add(key, value)
    storage[key] = value
end

function rune.alias.remove(key)
    if storage[key] then
        storage[key] = nil
        rune.print("[Alias] Removed: " .. key)
    else
        rune.print("[Alias] Not found: " .. key)
    end
end

function rune.alias.list()
    rune.print("[Aliases]")
    local count = 0
    for k, v in pairs(storage) do
        if type(v) == "function" then
            rune.print("  " .. k .. " -> (function)")
        else
            rune.print("  " .. k .. " -> " .. v)
        end
        count = count + 1
    end
    if count == 0 then
        rune.print("  (none)")
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
        on_input(expansion)
    end
    return true
end
