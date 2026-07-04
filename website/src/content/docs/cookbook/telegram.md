---
title: Forward Tells to Telegram
description: Get pinged on your phone when someone tells you in-game, using the built-in HTTP API.
---

Forward tells to Telegram for the times you're logged in but away from the
keyboard. One-time setup: create a bot with `@BotFather`, get your chat id
from `@userinfobot`, and export `TELEGRAM_TOKEN` and `TELEGRAM_CHAT` in
your shell.

```lua
local TOKEN = os.getenv("TELEGRAM_TOKEN") or rune.store.get("telegram_token")
local CHAT  = os.getenv("TELEGRAM_CHAT")  or rune.store.get("telegram_chat")

-- Percent-encode a value for a form body. Tell text is arbitrary
-- game text; encoding makes it inert.
local function urlencode(s)
    return (s:gsub("[^%w%-%.%_%~]", function(c)
        return string.format("%%%02X", string.byte(c))
    end))
end

local function telegram(text)
    if not TOKEN or not CHAT then return end
    rune.http.post(
        "https://api.telegram.org/bot" .. TOKEN .. "/sendMessage",
        "chat_id=" .. urlencode(CHAT) .. "&text=" .. urlencode(text),
        { headers = { ["Content-Type"] = "application/x-www-form-urlencoded" },
          timeout = 10 },
        function(resp, err)
            if err then
                rune.echo("[telegram] " .. err)
            elseif resp.status ~= 200 then
                rune.echo("[telegram] HTTP " .. resp.status)
            end
        end)
end

rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m)
    telegram(m[1] .. ": " .. m[2])
end, { name = "tells-to-telegram", group = "telegram" })
```

## How it works

- **[`rune.http.post`](/reference/api/http/) is asynchronous.** The
  request runs off the client's event loop and the callback runs back on
  it, so a slow network never freezes the UI — no shell, no `curl`, and
  it works the same on Windows.
- **`urlencode` is still required.** Tell text is attacker-controlled
  input going into a form body; percent-encoding keeps a hostile
  player's `&chat_id=...` from becoming structure.
- **The callback reports failures** instead of losing them. Drop it for
  true fire-and-forget.
- **Secrets:** the environment variable is the better home for the token,
  since `store.json` is plaintext on disk. The `rune.store` fallback is a
  convenience; use it knowing the trade-off.
- **`/group telegram off`** when you're back at the keyboard.

## Variations

Arm it automatically when you go idle:

```lua
local idle
rune.hooks.on("input", function()
    rune.group.disable("telegram")
    if idle then idle:remove() end
    idle = rune.timer.after(300, function()
        rune.group.enable("telegram")
    end)
end, { priority = 10 })
```

Forward to any webhook the same way — Discord, Slack, ntfy.sh — by
swapping the URL and body format (JSON bodies want a
`Content-Type = "application/json"` header).

**Related:** [rune.http reference](/reference/api/http/) · [Triggers](/scripting/triggers/) · [Hooks & Events](/scripting/hooks/) · [Groups](/scripting/groups/) · [Timers](/scripting/timers/)
