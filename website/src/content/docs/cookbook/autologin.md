---
title: Auto-Login with Worlds
description: Store a character name on each world bookmark and answer the login prompt automatically, without putting passwords in plaintext.
---

World bookmarks accept extra fields, so you can store the character per
world. Run this once (from `/lua` or `init.lua`; worlds persist in
[`rune.store`](/scripting/storage/)):

```lua
rune.world.add("viking", "vikingmud.org:2001", { character = "Ragnar" })
```

Then answer the login prompt from the bookmark of whatever you connected to:

```lua
local pending  -- the character for the world we're dialing

rune.hooks.on("connecting", function(addr)
    pending = nil
    for _, w in ipairs(rune.world.list()) do
        local entry = rune.world.get(w.name)
        if entry.address == addr and entry.character then
            pending = entry.character
        end
    end
end)

rune.trigger.contains("What is your name", function()
    if pending then
        rune.send(pending)
        pending = nil   -- fire once per connection
    end
end)
```

## How it works

- The `"connecting"` hook receives the address being dialed.
  `rune.world.list()` returns only `{name, address}`, so the loop calls
  `rune.world.get()` for the full entry with the extra fields.
- Clearing `pending` after sending keeps the trigger inert if the phrase
  shows up again mid-session.

## Passwords

Two options, in order of preference.

**Read from the environment (recommended):** keep the password in your
system keychain or an environment variable, never in Lua or `store.json`:

```lua
rune.trigger.contains("Password:", function()
    local pw = os.getenv("MUD_PASSWORD")
    if pw then rune.send_raw(pw) end
end)
```

`send_raw` skips command expansion, so a password containing `;` or `#`
arrives intact.

**Type it yourself:** don't automate the password line at all. The client
already suppresses local echo while the server hides input, so nothing
lands in your session log either way.

Avoid `rune.store.set("password", ...)`. `store.json` is plaintext on
disk, and the convenience isn't worth it.
