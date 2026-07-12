# The README montage: one unbroken session touching every headline
# feature - world picker, tab completion, history search, triggers'
# world (combat), and the verbatim composer.
Output out/raw/montage.mp4

Hide
Type "XDG_CONFIG_HOME=$DEMO_CONFIG $RUNE_BIN"
Enter
Sleep 1.5s
Show

Sleep 600ms

Type "/connect"
Sleep 400ms
Enter
Sleep 1.2s
Type "arc"
Sleep 700ms
Enter
Sleep 2.8s

# South to the shore, where the room teaches kraken / krill / Kralk.
Type "s"
Sleep 200ms
Enter
Sleep 2.4s

Type "tell kr"
Sleep 1s
# OVERLAY Tab|complete words the server has used
Tab
Sleep 900ms
Tab
Sleep 900ms
Tab
Sleep 800ms
Type "how much for the axe?"
Sleep 300ms
Enter
Sleep 2.4s

# OVERLAY Ctrl+R|search command history
Ctrl+R
Sleep 1.1s
Type "kil"
Sleep 900ms
Enter
Sleep 700ms
Enter
Sleep 3.6s
Sleep 1.4s

# Knubo wants the patrol report on the board.
Type "note write"
Sleep 200ms
Enter
Sleep 1.2s

Type "Two fresh kills on the south shore."
Sleep 1s
# OVERLAY Ctrl+Enter|grow the input into a composer
Ctrl+J
Sleep 700ms
Type "The kraken is circling the seal hole again."
Ctrl+J
Sleep 500ms
Type "Do not cross the rope bridge after dark."
Sleep 1.6s

Enter
Sleep 1s
Type "."
Sleep 200ms
Enter
Sleep 3s

Sleep 1.5s

Hide
Type "/quit"
Enter
