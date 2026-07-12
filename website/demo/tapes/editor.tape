# Editor mode: Ctrl+E hands the draft to $EDITOR; a multi-line result
# comes back in the verbatim composer, ready to send.
Output out/raw/editor.mp4

Hide
Type "EDITOR=vim XDG_CONFIG_HOME=$DEMO_CONFIG $RUNE_BIN arctic"
Enter
Sleep 2s
Show

# Tundra's tell lands ~2.5s after connect (scenario cue).
Sleep 2.4s

Type "mail tundra"
Sleep 300ms
Enter
Sleep 1.8s

# OVERLAY Ctrl+E|write the letter in $EDITOR
Ctrl+E
Sleep 1.5s

# Inside vim: a letter that actually earns an editor.
Type "i"
Sleep 400ms
Type@40ms "Tundra,"
Enter
Enter
Type@40ms "Meet at the icefall at dusk. Bring rope, torches,"
Enter
Type@40ms "and the sled. Bait for the seal hole too."
Enter
Enter
Type@40ms "If the kraken stirs, we run. No heroics."
Sleep 900ms
Escape
Sleep 400ms
Type ":wq"
Sleep 500ms
Enter
Sleep 2.2s

# Back in the client: the whole letter sits in the composer.
# OVERLAY Enter|send it exactly as written
Enter
Sleep 1.4s
Type "."
Sleep 300ms
Enter
Sleep 3.4s

Sleep 1.6s

Hide
Type "/quit"
Enter
