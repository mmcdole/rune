-- JSON syntax and string/number conversion live in Go. Lua owns container
-- identity: private weak keys preserve array/object intent without changing
-- user metatables or adding fields to their data.
local native = rune._json
local null = native.null
local kinds = setmetatable({}, { __mode = "k" })
local next, rawget, type, rawequal = next, rawget, type, rawequal
local max_depth, max_bytes, max_nodes = native.max_depth, native.max_bytes, native.max_nodes

rune.json = { null = null }

local function mark(value, kind)
    if value == nil then value = {} end
    if type(value) ~= "table" then
        error("rune.json." .. kind .. ": table expected", 3)
    end
    kinds[value] = kind
    return value
end

function rune.json.array(value)
    return mark(value, "array")
end

function rune.json.object(value)
    return mark(value, "object")
end

-- The private wire form wraps only containers and null: {kind, contents}.
-- Scalars pass through unchanged. Every user table is wrapped, so member
-- names can never be mistaken for type metadata. Both VM backends already
-- support these plain trees; their general-purpose conversion stays intact.
function rune.json.encode(value)
    local seen, nodes, bytes = {}, 0, 0
    local function pack(v, depth, path)
        nodes = nodes + 1
        if nodes > max_nodes then return nil, path .. ": JSON exceeds " .. max_nodes .. " values" end
        local kind = type(v)
        bytes = bytes + (kind == "string" and #v + 2 or 1)
        if bytes > max_bytes then return nil, path .. ": JSON exceeds 5 MiB" end
        if v == nil or rawequal(v, null) then return { "null" } end
        if kind ~= "table" then
            if kind == "string" or kind == "number" or kind == "boolean" then return v end
            return nil, path .. ": cannot encode " .. kind
        end
        if depth >= max_depth then return nil, path .. ": JSON exceeds " .. max_depth .. " container levels" end
        if seen[v] then return nil, path .. ": reference cycle" end
        seen[v] = true
        local count, numeric, highest = 0, 0, 0
        for key in next, v do
            count = count + 1
            if count > max_nodes then return nil, path .. ": JSON exceeds " .. max_nodes .. " values" end
            if type(key) == "number" and key >= 1 and key % 1 == 0 then
                numeric = numeric + 1
                if key > highest then highest = key end
            elseif type(key) ~= "string" then
                return nil, path .. ": keys must be strings or positive integers"
            end
        end
        local shape = kinds[v] or (numeric > 0 and "array" or "object")
        if (shape == "array" and (numeric ~= count or highest ~= count)) or
           (shape == "object" and numeric ~= 0) then
            return nil, path .. ": mixed keys, sparse array, or keys incompatible with " .. shape
        end
        local contents = {}
        for key, child in next, v do
            if type(key) == "string" then bytes = bytes + #key + 3 end
            if bytes > max_bytes then return nil, path .. ": JSON exceeds 5 MiB" end
            local child_path = path .. "[" .. (type(key) == "string" and string.format("%q", key) or key) .. "]"
            local packed, err = pack(child, depth + 1, child_path)
            if err then return nil, err end
            contents[key] = packed
        end
        seen[v] = nil
        return { shape, contents }
    end
    local packed, err = pack(value, 0, "$")
    if err then return nil, err end
    return native.encode(packed)
end

function rune.json.decode(text)
    if type(text) ~= "string" then error("rune.json.decode: string expected", 2) end
    local packed, err = native.decode(text)
    if err then return nil, err end
    local function unpack(v)
        if type(v) ~= "table" then return v end
        local kind = rawget(v, 1)
        if kind == "null" then return null end
        local result = rawget(v, 2)
        for key, child in next, result do result[key] = unpack(child) end
        kinds[result] = kind
        return result
    end
    return unpack(packed)
end
