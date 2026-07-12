# The verbatim composer: Ctrl+Enter grows the input into a lossless
# multiline draft; Enter sends it exactly as written.
Output out/raw/verbatim.mp4

Hide
Type "XDG_CONFIG_HOME=$DEMO_CONFIG $RUNE_BIN arctic"
Enter
Sleep 2s
Show

# Knubo's tell lands ~2.2s after connect (scenario cue).
Sleep 1.5s

Type "note write"
Sleep 300ms
Enter
Sleep 1.4s

Type "Two fresh kills on the south shore."
Sleep 1.2s
# OVERLAY Ctrl+Enter|grow the input into a composer
Ctrl+J
Sleep 900ms
Type "The kraken is circling the seal hole again."
Ctrl+J
Sleep 700ms
Type "Do not cross the rope bridge after dark."
Sleep 2s

# OVERLAY Enter|send every line as written
Enter
Sleep 1.2s
Type "."
Sleep 300ms
Enter
Sleep 3s

Sleep 2s

Hide
Type "/quit"
Enter
