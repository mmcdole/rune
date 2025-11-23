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

-- The Recursive Executor
local function process_queue(queue)
    if #queue == 0 then return end

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

            return process_queue(queue)
        end
        -- Continue with rest of queue after alias
        process_queue(queue)
        return
    end

    -- 3. Normal Command - send directly to network
    rune.send_raw(current)

    -- 4. Continue with rest of queue
    process_queue(queue)
end

-- Hook called by Go
function on_input(input)
    -- Check for slash command first
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        local handler = rune.command.get(cmd)
        if handler then
            handler(args)
        else
            rune.print("[Error] Unknown command: /" .. cmd)
        end
        return
    end

    -- Preprocess TinTin++ syntax (e.g., #6 north)
    if rune.tintin and rune.tintin.expandRepeats then
        input = rune.tintin.expandRepeats(input)
    end

    -- Process as normal command
    local queue = parse_to_queue(input)
    process_queue(queue)
end

-- Hook called by Go
function on_output(line)
    -- Process triggers
    local modified, show = rune.trigger.process(line)
    if not show then
        return nil  -- gag the line
    end
    return modified
end

-- Hook called by Go for prompts
function on_prompt(text)
    -- Process triggers against prompt text too
    local modified, show = rune.trigger.process(text)
    if not show then
        return ""  -- gag the prompt
    end
    return modified
end
