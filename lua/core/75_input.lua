-- Input processing.
-- Session processes one submitted line, echoes the result, then executes it.
-- Session owns splitting submissions into lines and recording their history.
-- Programmatic rune.send executes game commands without history expansion or input hooks.

-- Send literal game text, splitting LF, CRLF, and bare CR into physical lines.
-- Bypasses history expansion, input hooks, aliases, repeats, and slash commands.
-- Returns true on success; on send failure, echoes the error and returns nil
-- plus its message. Stops at the first failed physical line.
function rune.send_raw(text)
    if type(text) == "string" and text:find("[\r\n]") then
        text = text:gsub("\r\n", "\n"):gsub("\r", "\n")
        local ok, err
        for line in (text .. "\n"):gmatch("(.-)\n") do
            ok, err = rune.send_raw(line)
            if not ok then
                return ok, err
            end
        end
        return ok, err
    end
    local ok, err = rune._send_raw(text)
    if not ok then
        rune.echo(rune.style.red("[Error]") .. " " .. tostring(err))
    end
    return ok, err
end

-- Input history (Go owns storage so it survives reloads)

rune.history = {}

-- Return history text oldest-first, including multiline and Verbatim entries.
-- Mode metadata stays in Go; this public API returns only the text.
function rune.history.get()
    local entries = rune._history.entries()
    local history = {}
    for i, entry in ipairs(entries) do
        history[i] = entry.text
    end
    return history
end

-- Record Command-mode text without sending it or running input processing.
-- Go validates the text and applies the normal history storage rules.
function rune.history.add(cmd)
    rune._history.add(cmd)
end

-- Iterate separator-delimited commands, yielding source text and decoded text.
-- A doubled separator is literal: with ";", "say a;;b;look" first yields
-- "say a;;b", "say a;b". Empty segments are preserved. History uses the source
-- spelling so replay does not turn literal separators into command boundaries.
local function commands(text, separator)
    local start = 1
    local doubled = separator .. separator
    local escaped_pattern = doubled:gsub("([^%w])", "%%%1")

    return function()
        if not start then return nil end

        local cursor = start
        while true do
            local pos = text:find(separator, cursor, true)
            if pos and text:sub(pos, pos + #doubled - 1) == doubled then
                cursor = pos + #doubled
            else
                local source = pos and text:sub(start, pos - 1) or text:sub(start)
                start = pos and pos + #separator or nil
                local command = source:gsub(escaped_pattern, function() return separator end)
                return source, command
            end
        end
    end
end

-- Interactive history expansion.
--
-- Called once before input hooks. Programmatic sends and hook rewrites never
-- re-enter expansion. Keep source spelling intact for echo, history, and replay.

-- Parse a history reference occupying a whole command segment. With "!":
-- "!" and "!!" return "" (latest eligible line); "!nor" returns "nor".
-- Return nil for ordinary text, including "say !!" or an escaped separator.
local function parse_history_reference(piece, marker, separator)
    local token = piece:match("^%s*(.-)%s*$")
    if token:sub(1, #separator) == separator or token:sub(1, #marker) ~= marker then
        return nil
    end

    local prefix = token:sub(#marker + 1)
    if prefix:find("%s") then
        return nil
    end
    if prefix == marker then
        return "" -- A doubled marker is equivalent to a bare marker.
    end
    return prefix
end

-- Return whether any command segment is a history reference. This also guards
-- against replaying unresolved references stored by rune.history.add or under
-- a different history character.
local function has_history_reference(text, separator, marker)
    if not text:find(marker, 1, true) then
        return false
    end

    for piece in commands(text, separator) do
        if parse_history_reference(piece, marker, separator) ~= nil then
            return true
        end
    end
    return false
end

-- Find the newest eligible physical history line matching prefix; "" matches
-- any prefix. Skip Verbatim entries, blank lines, slash-command lines, and lines
-- containing unresolved history references. Return the original line, which may
-- contain multiple separator-delimited commands, or nil when nothing matches.
local function find_previous(prefix, history, separator, marker)
    for i = #history, 1, -1 do
        local entry = history[i]
        if entry.mode == "command" then
            -- A history entry can hold a batch. Expansion selects its most
            -- recent eligible command line; history navigation recalls the block.
            local lines = {}
            local normalized = entry.text:gsub("\r\n", "\n"):gsub("\r", "\n")
            for line in (normalized .. "\n"):gmatch("(.-)\n") do
                lines[#lines + 1] = line
            end
            for j = #lines, 1, -1 do
                local candidate = lines[j]:match("^%s*(.-)%s*$")
                if candidate ~= ""
                    and lines[j]:sub(1, 1) ~= "/"
                    and not has_history_reference(lines[j], separator, marker)
                    and (prefix == "" or candidate:sub(1, #prefix) == prefix)
                then
                    return lines[j]
                end
            end
        end
    end
end

-- Replace history-reference segments, retaining the source spelling elsewhere.
-- Return the rebuilt text; if any reference has no match, print a diagnostic
-- and return false so the whole input line is skipped before hooks or sending.
local function expand_commands(text, history, separator, marker)
    local pieces = {}
    for piece in commands(text, separator) do
        local prefix = parse_history_reference(piece, marker, separator)
        if prefix ~= nil then
            local replacement = find_previous(prefix, history, separator, marker)
            if not replacement then
                local token = piece:match("^%s*(.-)%s*$")
                rune.echo(rune.style.yellow("[History]") ..
                    " no matching command: " .. token)
                return false
            end
            pieces[#pieces + 1] = replacement
        else
            pieces[#pieces + 1] = piece
        end
    end

    -- A recalled line may start or end with separators. Keep them from
    -- pairing with the separator that joins it to the surrounding commands.
    for i = 2, #pieces do
        if pieces[i - 1]:sub(-#separator) == separator
            or pieces[i]:sub(1, #separator) == separator
        then
            pieces[i - 1] = pieces[i - 1] .. " "
            pieces[i] = " " .. pieces[i]
        end
    end
    return table.concat(pieces, separator)
end

-- Expand history references in an interactive Command-mode line. Slash commands
-- and disabled expansion pass through unchanged. Return text, or false if a
-- reference has no match. Called before hooks, never again for their rewrites.
local function expand_history(text)
    if text:sub(1, 1) == "/" then return text end
    local marker = rune.config.get("history_character")
    if marker == "" or not text:find(marker, 1, true) then return text end

    local separator = rune.config.get("command_separator")
    if has_history_reference(text, separator, marker) then
        return expand_commands(text, rune._history.entries(), separator, marker)
    end
    return text
end

-- Command sequences, single-command repeats, and alias expansion.

local MAX_RECURSION_DEPTH = 100

-- Execute a game-command sequence: split separators, decode literal separators,
-- apply #N repeats, then try aliases before sending each command. Alias text is
-- processed recursively; an alias returning no text consumes the command.
-- This does not interpret slash commands or perform interactive preparation.
local function execute_commands(input, depth)
    if depth > MAX_RECURSION_DEPTH then
        rune.echo(rune.style.red("[Error]") .. " Alias loop detected (depth limit exceeded)")
        return
    end

    local separator = rune.config.get("command_separator")
    for source, command in commands(input, separator) do
        command = command:match("^%s*(.-)%s*$")
        -- Recognize the prefix before decoding can introduce a literal '#'.
        local count = source:match("^%s*#(%d+)%s+%S")
        local line = count and command:match("^#%d+%s+(.-)%s*$") or command

        for _ = 1, tonumber(count) or 1 do
            if line == "" then
                rune.send_raw(line)
            else
                local processed, result = rune.alias.process(line)
                if not processed then
                    rune.send_raw(line)
                elseif result then
                    execute_commands(result, depth + 1) -- alias result is new command text
                end
            end
        end
    end
end

-- Send programmatic game-command text through separators, repeats, and aliases.
-- Does not run input hooks, expand history, echo input, record history, or dispatch
-- slash commands. Send failures are reported by send_raw; no status is returned.
function rune.send(input)
    execute_commands(input, 0)
end

-- Process one submitted line: expand history in Command mode, then
-- run input hooks in either mode. Return text, or false to skip this line.
-- Does not send, echo input, or record history; Session owns those steps.
function rune.input._process_submitted_line(text, mode)
    if mode == "command" then
        text = expand_history(text)
        if text == false then return false end
    end
    -- A missing hook system still permits recovery commands and literal sends.
    if type(rune.hooks) ~= "table" or type(rune.hooks.call) ~= "function" then
        return text
    end
    return rune.hooks.call("input", text, { mode = mode })
end

-- Execute one processed line: Verbatim sends literally; Command mode
-- runs a leading slash command or executes game-command syntax.
-- No input hooks, history expansion, input echo, or history recording here.
function rune.input._execute_input_line(text, mode)
    if mode == "verbatim" then
        rune.send_raw(text) -- no alias or command interpretation
        return
    end

    -- Check for slash command first. Dispatch runs the handler under
    -- its own quarantine, so a broken command is disabled individually
    -- instead of breaking the terminal dispatcher.
    local cmd, args = text:match("^/(%S+)%s*(.*)")
    if cmd then
        if not rune.command.dispatch(cmd, args) then
            rune.echo(rune.style.red("[Error]") .. " Unknown command: /" .. cmd)
        end
        return
    end

    rune.send(text)
end
