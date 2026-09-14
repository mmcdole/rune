-- INTERNAL: Route one physical line after input hooks and local echo.
-- Programmatic rune.send deliberately enters below this boundary.
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
