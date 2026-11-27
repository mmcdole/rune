-- Input
-- The input line and what happens to a submitted line: rune.input, command
-- syntax (";" separators, #N repeats, "!" history references), alias and
-- slash-command dispatch, and sending.
--
-- Go owns the input widget (rune._input), splits a submission into lines,
-- and echoes and records each; this file owns each line's processing.
--
-- API:
--   rune.input.get()  rune.input.set(text)
--   rune.input.get_cursor()  rune.input.set_cursor(pos)  -- UTF-8 byte offsets
--   rune.input.word_left()  rune.input.word_right()  rune.input.delete_word()
--   rune.input.open_editor(initial)   -- $EDITOR; edited_text, ok
--   rune.send(text)                   -- command syntax and aliases, then send
--   rune.send_raw(text)               -- literal game lines
--
-- Notes:
--   One top-level send (a typed line, a trigger or timer action) may nest
--   aliases 100 deep and run 1000 commands; past either, the whole
--   expansion stops with one error. "!" history expansion runs once per
--   typed line, before input hooks; history navigation is history.lua.

rune.input = {}

-- The input line

function rune.input.get()
    return rune._input.get()
end

function rune.input.set(text)
    rune._input.set(text)
end

function rune.input.get_cursor()
    return rune._input.get_cursor()
end

function rune.input.set_cursor(pos)
    rune._input.set_cursor(pos)
end

-- Open $EDITOR with the given initial text.
-- Returns edited_text, ok.
function rune.input.open_editor(initial)
    return rune._input.open_editor(initial)
end

-- Find previous word boundary
local function find_word_left(text, pos)
    if pos <= 0 then return 0 end
    local new_pos = pos
    -- Skip spaces (going backwards)
    while new_pos > 0 and text:sub(new_pos, new_pos) == " " do
        new_pos = new_pos - 1
    end
    -- Skip word characters
    while new_pos > 0 and text:sub(new_pos, new_pos) ~= " " do
        new_pos = new_pos - 1
    end
    return new_pos
end

-- Find next word boundary
local function find_word_right(text, pos)
    local len = #text
    if pos >= len then return len end
    local new_pos = pos + 1
    -- Skip current word characters
    while new_pos <= len and text:sub(new_pos, new_pos) ~= " " do
        new_pos = new_pos + 1
    end
    -- Skip spaces
    while new_pos <= len and text:sub(new_pos, new_pos) == " " do
        new_pos = new_pos + 1
    end
    return new_pos - 1
end

-- Word left: move cursor to previous word boundary
function rune.input.word_left()
    local text = rune.input.get()
    local pos = rune.input.get_cursor()
    local new_pos = find_word_left(text, pos)
    rune.input.set_cursor(new_pos)
end

-- Word right: move cursor to next word boundary
function rune.input.word_right()
    local text = rune.input.get()
    local pos = rune.input.get_cursor()
    local new_pos = find_word_right(text, pos)
    rune.input.set_cursor(new_pos)
end

-- Delete word before cursor
function rune.input.delete_word()
    local text = rune.input.get()
    local pos = rune.input.get_cursor()
    if pos <= 0 then return end

    local new_pos = find_word_left(text, pos)
    local before = text:sub(1, new_pos)
    local after = text:sub(pos + 1)
    rune.input.set(before .. after)
    rune.input.set_cursor(new_pos)
end

-- Sending

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

local MAX_DEPTH = 100     -- nested alias expansions
local MAX_COMMANDS = 1000 -- commands one top-level send may run

-- The expansion in progress. One top-level send (a typed line, a trigger or
-- timer action) owns the depth and command budget; a rune.send made while it
-- runs, e.g. from an alias function, joins it instead of starting fresh.
local expansion

local function stop_expansion(message)
    expansion.stopped = true
    rune.echo(rune.style.red("[Error]") .. " " .. message)
end

-- Execute a game-command sequence: split separators, decode literal separators,
-- apply #N repeats, then try aliases before sending each command. Alias text is
-- processed recursively; an alias returning no text consumes the command.
-- This does not interpret slash commands or perform interactive preparation.
local function execute_commands(input)
    local state = expansion
    if state.depth > MAX_DEPTH then
        return stop_expansion("Alias loop detected (more than " .. MAX_DEPTH .. " nested expansions)")
    end

    local separator = rune.config.get("command_separator")
    for source, command in commands(input, separator) do
        if state.stopped then return end
        command = command:match("^%s*(.-)%s*$")
        -- Recognize the prefix before decoding can introduce a literal '#'.
        local count = source:match("^%s*#(%d+)%s+%S")
        local line = count and command:match("^#%d+%s+(.-)%s*$") or command
        local repeats = tonumber(count) or 1
        if repeats > state.remaining then
            return stop_expansion("Too many commands (limit " .. MAX_COMMANDS .. " per input)")
        end

        for _ = 1, repeats do
            if state.stopped then return end
            state.remaining = state.remaining - 1
            local processed, result = false, nil
            if line ~= "" then
                state.depth = state.depth + 1
                processed, result = rune.alias.process(line)
                if processed and result and not state.stopped then
                    execute_commands(result) -- alias result is new command text
                end
                state.depth = state.depth - 1
            end
            if not processed and not rune.send_raw(line) then
                state.stopped = true -- send_raw already echoed the failure
            end
        end
    end
end

-- Send programmatic game-command text through separators, repeats, and aliases.
-- Does not run input hooks, expand history, echo input, record history, or dispatch
-- slash commands. Send failures are reported by send_raw; no status is returned.
function rune.send(input)
    if expansion then
        if not expansion.stopped then
            execute_commands(input)
        end
        return
    end
    expansion = { depth = 0, remaining = MAX_COMMANDS, stopped = false }
    local ok, err = pcall(execute_commands, input)
    expansion = nil
    if not ok then
        error(err, 0)
    end
end

-- Process one submitted line: expand history in Command mode, then
-- run input hooks in either mode. `hidden` is true while the server hides
-- input (a password prompt). Return text, or false to skip this line.
-- Does not send, echo input, or record history; Session owns those steps.
function rune.input._process_submitted_line(text, mode, hidden)
    if mode == "command" then
        text = expand_history(text)
        if text == false then return false end
    end
    -- A missing hook system still permits recovery commands and literal sends.
    if type(rune.hooks) ~= "table" or type(rune.hooks.call) ~= "function" then
        return text
    end
    return rune.hooks.call("input", text, { mode = mode, hidden = hidden == true })
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

-- Defaults

-- Local echo styling. Re-register "echo-style" to restyle echoed input
-- (Go keeps a plain fallback for when the Lua core is unavailable).
rune.hooks.on("echo", function(text)
    return rune.style.green("> " .. text)
end, { name = "echo-style", priority = 100 })

rune.bind("ctrl+u", function() rune.input.set("") end)

-- /raw <text> - Send without alias expansion
rune.command.add("raw", function(args)
    if args == "" then
        rune.echo("[Usage] /raw <text>")
        return
    end
    rune.send_raw(args)
end, "Send text without alias expansion")
