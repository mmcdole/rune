#!/usr/bin/env bash
# Render the demo recordings. Each tape gets a fresh demo MUD server
# (per-scenario cues) and a fresh rune profile, so every run is
# deterministic. Only the README montage is published; the other tapes
# render to out/. Requires: vhs, ttyd, ffmpeg, vim, go.
#
#   ./render.sh            # the README montage
#   ./render.sh verbatim   # one tape
set -euo pipefail
cd "$(dirname "$0")"

WORK="${DEMO_WORK:-$(mktemp -d)}"
export RUNE_BIN="$WORK/rune"
MUD_BIN="$WORK/demomud"

go build -o "$RUNE_BIN" ../../cmd/rune/
go build -o "$MUD_BIN" ./mud/

# The tapes seed worlds/aliases/history from this profile. Copy it so
# store.json writes (worlds, /reconnect) never dirty the repo.
export DEMO_CONFIG="$WORK/config"
rm -rf "$DEMO_CONFIG"
cp -r config "$DEMO_CONFIG"

mkdir -p out out/raw
tapes=("$@")
[ ${#tapes[@]} -eq 0 ] && tapes=(montage)

for name in "${tapes[@]}"; do
  echo "=== $name ==="
  pkill -f "demomud" 2>/dev/null || true
  sleep 0.3
  # The demo worlds carry real-looking addresses; RUNE_DIAL_OVERRIDES
  # (a client dev seam, see network/client.go) reroutes them to the
  # local demo server. Nothing outside this process is touched, and the
  # real hosts are never dialed. High ports to dodge local services.
  addr="127.0.0.1:42700"
  [ "$name" = hero ] && addr="127.0.0.1:42001"
  export RUNE_DIAL_OVERRIDES="vikingmud.org:2001=127.0.0.1:42001,mud.arctic.org:2700=127.0.0.1:42700"
  "$MUD_BIN" -addr "$addr" -scenario "$name" &
  MUD_PID=$!
  sleep 0.4
  # A dead server means the tape would record against whatever else
  # answers (or nothing). Refuse to continue.
  kill -0 "$MUD_PID" 2>/dev/null || { echo "demo server failed to bind $addr"; exit 1; }

  cat tapes/_header.tape "tapes/$name.tape" > "$WORK/$name.tape"
  vhs "$WORK/$name.tape"

  kill "$MUD_PID" 2>/dev/null || true

  # Key-press overlays (timed from the tape itself), final mp4 + gif.
  python3 overlay.py "$WORK/$name.tape" "out/raw/$name.mp4" \
    "out/$name.mp4" "out/$name.gif"
done
echo "done:" && ls -la out/*.gif out/*.mp4 2>/dev/null

# Publish: README montage into .github/.
for name in "${tapes[@]}"; do
  case "$name" in
    montage)
      cp out/montage.gif ../../.github/montage.gif
      ;;
  esac
done
