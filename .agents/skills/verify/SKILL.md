---
name: verify
description: Build and drive the rune TUI headlessly in tmux to verify changes at the real terminal surface.
---

# Verifying rune changes

rune is a full-screen bubbletea TUI; drive it in an isolated tmux server and
capture panes as evidence.

## Build and launch

```bash
SCRATCH=$(mktemp -d)
go build -o "$SCRATCH/rune" ./cmd/rune/
# Isolate config so the run never touches ~/.config/rune (init.lua, store.json)
tmux -L runeverify new-session -d -x 100 -y 30 "XDG_CONFIG_HOME=$SCRATCH/cfg $SCRATCH/rune"
```

## Drive and capture

```bash
tmux -L runeverify send-keys -l '/'          # -l = literal chars
tmux -L runeverify send-keys Tab             # named keys: Tab, Enter, Escape, BSpace, C-u
sleep 0.3                                    # let the Go<->Lua round trip land
tmux -L runeverify capture-pane -p | grep -v '^$' | tail -15
```

## Teardown

```bash
tmux -L runeverify send-keys C-u; tmux -L runeverify send-keys -l '/quit'; tmux -L runeverify send-keys Enter
tmux -L runeverify kill-server 2>/dev/null
```

## Gotchas

- **Sleep between keystrokes that trigger Lua binds.** Bound keys (e.g. `/` on
  empty input) round-trip UI -> Session -> Lua -> UI; sending a burst like
  `send-keys -l '/vers'` races the bind's `rune.input.set` and gives
  misleading state. Send the bound key, sleep ~0.3s, then continue.
- No server needed for UI/input/picker/command flows; `/version`, `/help`,
  `/echo` exercise dispatch while Disconnected. Avoid `/connect` to real MUDs.
- Useful flows: `/` opens the inline command picker; Tab completes; Esc
  cancels; ctrl+r opens the modal history picker; `/lua <expr>` evaluates
  Lua inline for state inspection.
