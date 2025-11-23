-- Command Processing System

-- Smart send that processes aliases and semicolons
rune.send = function(input)
    on_input(input)
end

-- Helper: Split string into a queue
local function parse_to_queue(input)
    local queue = {}
    -- Handle empty input - should send empty command
    if input == "" then
        return {""}
    end
    for part in string.gmatch(input, "[^"..rune.config.delimiter.."]+") do
        table.insert(queue, part:match("^%s*(.-)%s*$"))
    end
    return queue
end

-- The Iterative Executor (avoids stack overflow with deep alias chains)
local function process_queue(queue)
    while #queue > 0 do
        -- Pop first item
        local current = table.remove(queue, 1)

        -- 1. Check for #wait
        local wait_time = current:match("^"..rune.config.wait_cmd.."%s+(%d+%.?%d*)")
        if wait_time then
            rune.print("[System] Pausing for " .. wait_time .. "s...")
            rune.timer.after(tonumber(wait_time), function()
                process_queue(queue)
            end)
            return
        end

        -- 2. Check for Alias (with argument support)
        -- Split current into command and args
        local cmd, args = current:match("^(%S+)%s*(.*)")
        if not cmd then
            cmd = current
            args = ""
        end

        local alias_value = rune.alias.get(cmd)
        if alias_value then
            if type(alias_value) == "function" then
                -- Function alias: call with args
                alias_value(args)
            else
                -- String alias: expand and prepend to queue
                -- If there were args, append them to the expansion
                local expansion = alias_value
                if args ~= "" then
                    expansion = expansion .. " " .. args
                end
                -- Apply TinTin++ preprocessing to alias expansion
                if rune.tintin and rune.tintin.expandRepeats then
                    expansion = rune.tintin.expandRepeats(expansion)
                end
                local expanded_queue = parse_to_queue(expansion)

                for i = #expanded_queue, 1, -1 do
                    table.insert(queue, 1, expanded_queue[i])
                end
            end
            -- Continue loop to process expanded/remaining queue
        else
            -- 3. Normal Command - send directly to network
            rune.send_raw(current)
        end
    end
end

-- Register core input handler
rune.hooks.register("input", function(input)
    -- Check for slash command first
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        local handler = rune.command.get(cmd)
        if handler then
            handler(args)
        else
            rune.print("[Error] Unknown command: /" .. cmd)
        end
        return false  -- consumed
    end

    -- Preprocess TinTin++ syntax (e.g., #6 north)
    if rune.tintin and rune.tintin.expandRepeats then
        input = rune.tintin.expandRepeats(input)
    end

    -- Process as normal command
    local queue = parse_to_queue(input)
    process_queue(queue)
    return false  -- consumed
end, { priority = 100 })

-- Register core output handler
rune.hooks.register("output", function(line)
    -- Process triggers
    local modified, show = rune.trigger.process(line)
    if not show then
        return false  -- gag the line
    end
    return modified
end, { priority = 100 })

-- Register core prompt handler
rune.hooks.register("prompt", function(text)
    -- Process triggers against prompt text too
    local modified, show = rune.trigger.process(text)
    if not show then
        return false  -- gag the prompt
    end
    return modified
end, { priority = 100 })
