-- Bar Renderer System
-- Built on rune.registry (15_registry.lua). Renderers get the same
-- quarantine as every other callback: three consecutive failures
-- disable the bar instead of erroring 4x/second forever.
--
-- API:
--   rune.ui.bar(name, render_fn, opts?) -- Register a bar renderer
--   rune.bars.list()                    -- For /bars
--
-- render_fn receives the terminal width and returns a string or a
-- table {left, center, right}. Go calls rune.bars._render_all on its
-- tick and pushes the results to the UI; re-registering a name gives
-- the renderer a fresh start.

local by_bar = {} -- bar name -> data

local registry = rune.registry.new{
    kind = "bar",
    on_add = function(data)
        -- Upsert by bar name: re-registering replaces (and resets
        -- any failure streak, since the entry is fresh)
        local old = by_bar[data.bar]
        if old and old ~= data then
            old._handle:remove()
        end
        by_bar[data.bar] = data
    end,
    on_remove = function(data)
        if by_bar[data.bar] == data then
            by_bar[data.bar] = nil
        end
    end,
}

rune.bars = {}

-- rune.ui.bar lives on the rune.ui table (created by the UI layer
-- registration); defined here because this module owns the registry.
function rune.ui.bar(name, render_fn, opts)
    return registry:add({
        bar = name,
        renderer = render_fn,
        source = rune.caller_source(1),
    }, opts)
end

-- INTERNAL: called by Go on the render tick.
-- Returns { [name] = string | {left, center, right} } for active bars.
function rune.bars._render_all(width)
    local out = {}
    for _, data in ipairs(registry:items()) do
        if registry:active(data) then
            local label = 'Bar "' .. data.bar .. '"' ..
                (data.source and (" @" .. data.source) or "")
            local ok, result = rune.guarded_call(label, data, data.renderer, width)
            if ok and (type(result) == "string" or type(result) == "table") then
                out[data.bar] = result
            end
        end
    end
    return out
end

-- Management by name (registry name, not bar name)
function rune.bars.disable(name)
    return registry:disable(name)
end

function rune.bars.enable(name)
    return registry:enable(name)
end

function rune.bars.remove(name)
    return registry:remove(name)
end

-- List all bars - returns array of {bar, name, group, enabled, source}
function rune.bars.list()
    local result = {}
    for _, data in ipairs(registry:items()) do
        table.insert(result, {
            bar = data.bar,
            name = data.name,
            group = data.group,
            enabled = data.enabled,
            source = data.source,
        })
    end
    table.sort(result, function(a, b) return a.bar < b.bar end)
    return result
end

function rune.bars.count()
    return registry:count()
end

function rune.bars.clear()
    registry:clear()
end

function rune.bars.remove_group(group_name)
    return registry:remove_group(group_name)
end
