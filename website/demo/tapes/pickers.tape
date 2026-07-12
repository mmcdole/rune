# Pickers for everything: / commands, /connect worlds, Ctrl+R history,
# Ctrl+T aliases.
Output out/raw/pickers.mp4

Hide
Type "XDG_CONFIG_HOME=$DEMO_CONFIG $RUNE_BIN"
Enter
Sleep 1.5s
Show

Sleep 1s

# Bare /connect opens the world picker.
Type "/connect"
Sleep 500ms
Enter
Sleep 1.4s
Type "arc"
Sleep 900ms
Enter
Sleep 2.6s

# OVERLAY Ctrl+R|search command history
Ctrl+R
Sleep 1.2s
Type "kil"
Sleep 1.1s
Enter
Sleep 900ms
Enter
Sleep 3.4s

# OVERLAY Ctrl+T|browse your aliases
Ctrl+T
Sleep 1.4s
Down
Sleep 700ms
Down
Sleep 1.4s
Escape
Sleep 2.5s

Hide
Type "/quit"
Enter
