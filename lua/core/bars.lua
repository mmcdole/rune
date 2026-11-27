-- Bar Renderer System
-- Built on rune.registry (registry.lua). Renderers get the same
-- quarantine as every other callback: three consecutive failures
-- disable the bar instead of erroring 4x/second forever.
--
-- API:
--   rune.ui.bar(name, render_fn, opts?) -- Register a bar renderer
--   rune.bars.get(name)                 -- The bar's handle, or nil
--   rune.bars.list()                    -- For /bars
--
-- render_fn receives the terminal width and returns a string or a
-- table {left, center, right}. Go calls rune.bars._render_all on its
-- tick and pushes the results to the UI. The bar's layout name is its
-- registry name, so re-registering it replaces the old renderer (and
-- resets its failure streak, since the entry is fresh) and
-- rune.bars.disable("status") addresses the bar you see.

local registry = rune.registry.new{
    kind = "bar",
    on_change = rune._ui.bars_changed,
}

rune.bars = registry:expose({})

-- rune.ui.bar lives on the rune.ui table (created by the UI layer
-- registration); defined here because this module owns the registry.
function rune.ui.bar(name, render_fn, opts)
    if type(name) ~= "string" or name == "" then
        error("rune.ui.bar: name must be a non-empty string", 2)
    end
    if type(render_fn) ~= "function" then
        error("rune.ui.bar: render_fn must be a function", 2)
    end
    return registry:add({
        bar = name,
        action = render_fn,
        source = rune.caller_source(1),
    }, rune.registry.keyed_opts(name, opts, "rune.ui.bar"), "rune.ui.bar")
end

-- INTERNAL: called by Go on the render tick.
-- Returns { [name] = string | {left, center, right} } for active bars.
function rune.bars._render_all(width)
    local out = {}
    -- Snapshot: a renderer that (re)registers bars must not perturb
    -- this render pass.
    for _, data in ipairs(registry:snapshot()) do
        if registry:active(data) then
            local ok, result = registry:call(data, width)
            if ok and (type(result) == "string" or type(result) == "table") then
                out[data.bar] = result
            end
        end
    end
    return out
end

-- List all bars - returns array of {bar, name, group, enabled, source, core}
function rune.bars.list()
    local result = registry:list(function(row, data)
        row.bar = data.bar
    end)
    table.sort(result, function(a, b) return a.bar < b.bar end)
    return result
end
