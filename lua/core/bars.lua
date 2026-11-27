-- Bars
-- Status bars: a named renderer returns a string or {left, center, right}
-- for a bar slot in the layout.
--
-- Go calls rune.bars._render_all on its render tick and pushes the
-- results to the UI; this file owns the renderer registry.
--
-- API:
--   rune.ui.bar(name, render_fn, opts?)  -- render_fn(width)
--   rune.bars.list()                     -- {bar, ...} by bar name
--   + shared management by bar name (get, enable, disable, toggle, remove,
--     count, clear, remove_group); see registry.lua
--
-- Notes:
--   The bar's layout name is its registry name, so re-registering it
--   replaces the renderer and rune.bars.disable("status") addresses the
--   bar you see. A renderer that keeps failing is disabled, not retried
--   four times a second.

local green, red, yellow, cyan, gray = rune.style.green, rune.style.red, rune.style.yellow, rune.style.cyan, rune.style.gray

local registry = rune.registry.new{
    kind = "bar",
    on_change = rune._ui.bars_changed,
}

rune.bars = registry:expose({})

-- rune.ui.bar extends the rune.ui table (ui.lua) because this file owns
-- the bar registry.
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

-- List all bars - returns array of {bar, name, group, enabled, source, core}
function rune.bars.list()
    local result = registry:list(function(row, data)
        row.bar = data.bar
    end)
    table.sort(result, function(a, b) return a.bar < b.bar end)
    return result
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

-- Defaults

-- /bars - List all bar renderers
rune.command.add("bars", function()
    local bars = rune.bars.list()
    rune.echo(green("[Bars]") .. gray(" (" .. #bars .. " total)"))
    if #bars == 0 then
        rune.echo("  " .. gray("(none)"))
        return
    end
    for _, b in ipairs(bars) do
        local status = b.enabled and green("[on] ") or red("[off]")
        local group_str = b.group and ("  " .. cyan("<" .. b.group .. ">")) or ""
        local src_str = rune.registry._origin(b)
        rune.echo(string.format("  %s %-16s%s%s",
            status, yellow(b.bar), group_str, src_str))
    end
end, "List all bar renderers")
