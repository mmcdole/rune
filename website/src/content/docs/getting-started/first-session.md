---
title: Your First Session
description: Connect, bookmark a world, log the session, and find your way around.
---

## Connect

Straight from your shell:

```sh
rune mud.example.com 4000           # plain telnet
rune mud.example.com 4000 tls       # TLS
rune tls://mud.example.com:4000     # same thing
```

Or start `rune` empty and connect from inside:

```txt
/connect mud.example.com 4000
```

## Bookmark it

```txt
/world add example mud.example.com 4000
```

From now on, `/connect example` inside the client (or `rune example` from
your shell) connects by name. `/connect` with no arguments opens a picker
over your saved worlds, and `/reconnect` redials the last server, even
across restarts.

## Find your way around

- `/help` lists every command; `/` alone opens a fuzzy command picker.
- `Up`/`Down` navigate history with prefix matching: type `tell ` and press
  `Up` to cycle only through your previous tells. `Ctrl+R` searches history.
- `Tab` completes words the server has used recently.
- `PageUp`/`PageDown` or the mouse wheel scroll the output. The status bar
  shows `SCROLL (n new)` while you're off the bottom.
- `Ctrl+C` clears the input line; pressed twice on an empty line it quits.

The full tour of the input line (editing keys, `Ctrl+E` into `$EDITOR`,
the default keymap) is in [Input & History](/interface/input/).

## Log the session

```txt
/log start              # writes to ~/.config/rune/logs/<timestamp>.log
/log stop
```

The log is ANSI-stripped and reads like your screen: server output after
your triggers ran, plus what you typed. Passwords are kept out of it,
because the echo is suppressed while the server hides input.

## Next

When you're ready to make the client yours, head to
[Scripting Basics](/getting-started/scripting-basics/).
