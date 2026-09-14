-- Interactive line processing. Session owns line iteration, echo, and history.
-- Programmatic rune.send enters below preparation and local-command dispatch.

-- History resolves exactly once, before any user input handler.
function rune.input._prepare(text, mode)
    if mode == "command" then
        text = rune.input._expand_history(text)
        if text == false then return false end
    end
    -- A missing hook system still permits recovery commands and literal sends.
    if type(rune.hooks) ~= "table" or type(rune.hooks.call) ~= "function" then
        return text
    end
    return rune.hooks.call("input", text, { mode = mode })
end

function rune.input._dispatch(input, mode)
    if mode == "verbatim" then
        rune.send_raw(input) -- no alias or command interpretation
        return
    end

    -- Check for slash command first. Dispatch runs the handler under
    -- its own quarantine, so a broken command is disabled individually
    -- instead of breaking the terminal dispatcher.
    local cmd, args = input:match("^/(%S+)%s*(.*)")
    if cmd then
        if not rune.command.dispatch(cmd, args) then
            rune.echo(rune.style.red("[Error]") .. " Unknown command: /" .. cmd)
        end
        return
    end

    rune.send(input)
end
