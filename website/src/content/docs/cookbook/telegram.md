---
title: Forward Tells to Telegram
description: Get pinged on your phone when someone tells you in-game, with the shell-safety details handled.
---

Forward tells to Telegram for the times you're logged in but away from the
keyboard. One-time setup: create a bot with `@BotFather`, get your chat id
from `@userinfobot`, and export `TELEGRAM_TOKEN` and `TELEGRAM_CHAT` in
your shell.

```lua
local TOKEN = os.getenv("TELEGRAM_TOKEN") or rune.store.get("telegram_token")
local CHAT  = os.getenv("TELEGRAM_CHAT")  or rune.store.get("telegram_chat")

-- Quote untrusted text for POSIX sh. Game text goes into a shell
-- command here: without this, a hostile player could send you
-- '; rm -rf ~' as a tell.
local function sh_quote(s)
    return "'" .. s:gsub("'", "'\\''") .. "'"
end

local function telegram(text)
    if not TOKEN or not CHAT then return end
    local url = "https://api.telegram.org/bot" .. TOKEN .. "/sendMessage"
    -- Fire-and-forget: the trailing & backgrounds curl so a slow
    -- network never blocks the client.
    os.execute("curl -s -o /dev/null --max-time 10" ..
        " -d chat_id=" .. sh_quote(CHAT) ..
        " --data-urlencode text=" .. sh_quote(text) ..
        " " .. sh_quote(url) .. " >/dev/null 2>&1 &")
end

rune.trigger.regex("^(\\w+) tells you: (.+)$", function(m)
    telegram(m[1] .. ": " .. m[2])
end, { name = "tells-to-telegram", group = "telegram" })
```

## How it works

- **`sh_quote` is required.** Tell text is attacker-controlled input going
  into a shell command, and the single-quote escape makes it inert. Keep
  this habit for every script that shells out with game text.
- **The trailing `&` keeps the client responsive.** Lua runs on the
  client's event loop; a synchronous `curl` on a bad connection would
  freeze the UI until the script watchdog killed the trigger. Backgrounded,
  it never blocks.
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

Platform note: `sh`, `&`, and `/dev/null` are POSIX. On Windows, run rune
under WSL or adapt with `start /b curl ...`.

**Related:** [Triggers](/scripting/triggers/) · [Hooks & Events](/scripting/hooks/) · [Groups](/scripting/groups/) · [Timers](/scripting/timers/)
