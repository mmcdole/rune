# Tab completion that learns words from server output, cycling on the
# status line, Shift+Tab going backward.
Output out/raw/tab.mp4

Hide
Type "XDG_CONFIG_HOME=$DEMO_CONFIG $RUNE_BIN arctic"
Enter
Sleep 2s
Show

Sleep 1.4s

# Walk to the shore: the room text teaches kraken / krill / Kralk.
Type "s"
Sleep 300ms
Enter
Sleep 2.6s

Type "tell kr"
Sleep 1.3s
# OVERLAY Tab|complete words the server has used
Tab
Sleep 1.1s
Tab
Sleep 1.1s
Tab
Sleep 1.4s
# OVERLAY Shift+Tab|cycle backward
Shift+Tab
Sleep 1.1s
Shift+Tab
Sleep 1.2s

# Forward again to land on Kralk and ask about the axe.
Tab
Tab
Sleep 900ms
Type "how much for the axe?"
Sleep 400ms
Enter
Sleep 2.6s

Sleep 2.5s

Hide
Type "/quit"
Enter
