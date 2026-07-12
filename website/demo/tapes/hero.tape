# The landing-page hero, as real footage: connect to VikingMUD, walk the
# village, tab-complete a name, recall a tell, then open the quake-style
# chat console the demo profile implements in a few lines of Lua.
Output out/raw/hero.mp4

Hide
Type "XDG_CONFIG_HOME=$DEMO_CONFIG $RUNE_BIN"
Enter
Sleep 1.5s
Show

Sleep 800ms

Type "/connect"
Sleep 500ms
Enter
Sleep 1.3s
Type "vik"
Sleep 800ms
Enter
Sleep 3s

# South to the village road; Tundra's first tell follows.
Type "s"
Sleep 200ms
Enter
Sleep 3.2s

Type "tell tun"
Sleep 900ms
# OVERLAY Tab|complete names the server has used
Tab
Sleep 900ms
Type "on my way"
Sleep 300ms
Enter
Sleep 2.4s

# East toward the shore; the tide is turning.
Type "e"
Sleep 200ms
Enter
Sleep 2.6s

# Alias: two commands from one word.
Type "ws"
Sleep 300ms
Enter
Sleep 2s

# OVERLAY Ctrl+R|search command history
Ctrl+R
Sleep 1.2s
Type "on my"
Sleep 1s
Enter
Sleep 800ms
Enter
Sleep 2.6s

# OVERLAY `|chat console - a few lines of Lua
Type "`"
Sleep 4s
Sleep 6s
Sleep 3s

Hide
Type "`"
Type "/quit"
Enter
