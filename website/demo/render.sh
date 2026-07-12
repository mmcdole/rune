#!/usr/bin/env bash
# Render the website demo gifs. Each tape gets a fresh demo MUD server
# (per-scenario cues) and a fresh rune profile, so every run is
# deterministic. Requires: vhs, ttyd, ffmpeg, vim, go.
#
#   ./render.sh            # all tapes
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
[ ${#tapes[@]} -eq 0 ] && tapes=(pickers tab editor verbatim)

for name in "${tapes[@]}"; do
  echo "=== $name ==="
  pkill -f "demomud" 2>/dev/null || true
  sleep 0.3
  # The hero connects to the "viking" world: /etc/hosts points
  # vikingmud.org at 127.0.0.2, and an iptables DNAT rule rewrites
  # 127.0.0.2:2001 -> :2101 because a real service (the mud-agent
  # bridge) may own port 2001 on this machine. Everything else uses
  # arctic on 2700. Verify the demo server answers before recording -
  # without the hosts alias + DNAT rule, the hero tape would type into
  # the REAL VikingMUD login prompt.
  addr=":2700"
  if [ "$name" = hero ]; then
    addr="127.0.0.2:2101"
    sudo -n iptables -t nat -C OUTPUT -d 127.0.0.2 -p tcp --dport 2001 \
      -j DNAT --to-destination 127.0.0.2:2101 2>/dev/null || {
      echo "hero: missing DNAT 127.0.0.2:2001->2101 (see comment)"; exit 1; }
    getent hosts vikingmud.org | grep -q "^127.0.0.2" || {
      echo "hero: /etc/hosts must map vikingmud.org to 127.0.0.2"; exit 1; }
  fi
  "$MUD_BIN" -addr "$addr" -scenario "$name" &
  MUD_PID=$!
  sleep 0.4

  cat tapes/_header.tape "tapes/$name.tape" > "$WORK/$name.tape"
  vhs "$WORK/$name.tape"

  kill "$MUD_PID" 2>/dev/null || true

  # Key-press overlays (timed from the tape itself), final mp4 + gif.
  python3 overlay.py "$WORK/$name.tape" "out/raw/$name.mp4" \
    "out/$name.mp4" "out/$name.gif"
done
echo "done:" && ls -la out/*.gif out/*.mp4 2>/dev/null

# Publish: site videos + posters into public/, README montage into .github/.
for name in "${tapes[@]}"; do
  case "$name" in
    hero|pickers|verbatim|editor|tab)
      cp "out/$name.mp4" ../public/demos/
      # Poster = the frame that best sells the feature (what reduced-motion
      # visitors see). Times track the tapes; re-tune if pacing changes.
      case "$name" in
        pickers)  poster=(-ss 15.2) ;;   # alias picker open
        verbatim) poster=(-ss 14.5) ;;   # composer, three numbered lines
        editor)   poster=(-ss 16.7) ;;   # letter back in the composer
        tab)      poster=(-ss 7.5) ;;    # candidates cycling on status line
        *)        poster=(-sseof -1.5) ;; # hero: chat console open
      esac
      ffmpeg -y -loglevel error "${poster[@]}" -i "out/$name.mp4" \
        -frames:v 1 -q:v 3 "../public/demos/$name-poster.jpg"
      ;;
    montage)
      cp out/montage.gif ../../.github/montage.gif
      ;;
  esac
done
