-- Command Processing System

-- STATE
local active_queue = nil   -- The table of pending commands
local inject_cursor = 1    -- Where to insert new commands (1 = front)

-- CONFIG
local MAX_OPS = 5000       -- Prevent infinite loops

-- Forward declaration
local process_queue

-- HELPER: Parse string to list (handles separators and TinTin expansion)
local function parse_to_list(input)
    -- 1. TinTin Expansion (#N {})
    if rune.tintin and rune.tintin.expandRepeats then
        input = rune.tintin.expandRepeats(input)
    end

    -- 2. Split by delimiter (preserving empty commands)
    local list = {}
    if input == "" then return {""} end

    -- Manual split to preserve empty strings between delimiters
    local delimiter = rune.config.delimiter
    local start = 1
    while true do
        local pos = input:find(delimiter, start, true)
        if not pos then
            -- Last part (or only part if no delimiter found)
            local part = input:sub(start)
            table.insert(list, part:match("^%s*(.-)%s*$"))
            break
        end
        -- Extract part before delimiter
        local part = input:sub(start, pos - 1)
        table.insert(list, part:match("^%s*(.-)%s*$"))
        start = pos + #delimiter
    end
    return list
end

-- RUNE.SEND: The Gateway
-- If called during queue processing, inserts at cursor position
-- Otherwise starts new queue processing
rune.send = function(input)
    local new_cmds = parse_to_list(input)

    if active_queue then
        -- Insert at current cursor (preserves order A -> B -> C)
        for _, cmd in ipairs(new_cmds) do
            table.insert(active_queue, inject_cursor, cmd)
            inject_cursor = inject_cursor + 1
        end
    else
        -- Start new queue processing
        process_queue(new_cmds)
    end
end

-- PROCESS QUEUE: The Event Loop
local run_loop

run_loop = function(queue)
    active_queue = queue

    local ok, err = pcall(function()
        local ops = 0

        while #queue > 0 do
            ops = ops + 1
            if ops > MAX_OPS then error("Infinite alias loop detected") end

            -- Reset cursor for the NEW command we are about to process.
            -- Expansions for this command must happen at the front.
            inject_cursor = 1

            local line = table.remove(queue, 1)

            -- 1. Check for #wait
            local wait_time = line:match("^"..rune.config.wait_cmd.."%s+(%d+%.?%d*)")
            if wait_time then
                -- Release lock so user can type while waiting
                active_queue = nil
                rune.print("[System] Pausing for " .. wait_time .. "s...")
                rune.timer.after(tonumber(wait_time), function()
                    run_loop(queue)
                end)
                return -- Exit completely
            end

            -- 2. Check for Alias (with argument support)
            local cmd, args = line:match("^(%S+)%s*(.*)")
            cmd = cmd or line
            args = args or ""

            local alias = rune.alias.get(cmd)
            if alias then
                if type(alias) == "function" then
                    -- Execute function alias
                    -- rune.send() calls inside will use the global inject_cursor
                    alias(args)
                else
                    -- String alias: expand and insert via rune.send
                    local expansion = alias
                    if args ~= "" then
                        expansion = expansion .. " " .. args
                    end
                    rune.send(expansion)
                end
            else
                -- 3. Normal Command - send directly to network
                rune.send_raw(line)
            end
        end
    end)

    -- Cleanup - only runs if loop finished or errored (not if #wait returned early)
    active_queue = nil

    if not ok then
        rune.print("\027[31m[Error] " .. tostring(err) .. "\027[0m")
    end
end

process_queue = run_loop

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

    -- Process as normal command (TinTin expansion happens in parse_to_list)
    local queue = parse_to_list(input)
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
rune.hooks.register("prompt", function(line)
    -- Process triggers against prompt (line is now a Line object)
    local modified, show = rune.trigger.process(line)
    if not show then
        return false  -- gag the prompt
    end
    return modified
end, { priority = 100 })
