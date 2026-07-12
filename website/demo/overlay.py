#!/usr/bin/env python3
"""Composite key-press overlays onto a VHS recording.

The tapes are deterministic, so the moment every chord fires is computable
from the tape source alone. A line like

    # OVERLAY Ctrl+R|search command history

annotates the next timed key command; this script parses the (concatenated)
tape, derives each overlay's time window, draws a website-style keycap with
PIL, and runs ffmpeg twice: once for the final mp4, once for the gif.

Usage: overlay.py <tape> <raw.mp4> <out.mp4> <out.gif>
"""

import re
import subprocess
import sys
import tempfile
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

# Site palette (Landing.astro --vars).
SCRIM = (20, 21, 31, 140)          # rgba(20,21,31,0.55)
PANEL = (27, 29, 43, 255)          # --panel
EDGE = (42, 45, 66, 255)           # --panel-edge
FG = (230, 233, 245, 255)          # keycap text (site .c-bold)
AMBER = (224, 175, 104, 255)       # --amber label

MONO_BOLD = "/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Bold.ttf"
MONO = "/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Medium.ttf"

KEY_MS = 55          # per key-command press; matches Set TypingSpeed
LEAD = 1.0           # overlay appears this long before the chord fires
TAIL = 0.55          # ... and lingers this long after
FADE_IN = 0.2
FADE_OUT = 0.25


def parse_tape(path):
    """Return (events, size): overlay events with absolute visible-time."""
    type_speed = 0.055
    t = 0.0
    shown = True
    pending = None
    events = []
    width, height = 1280, 660

    key_re = re.compile(
        r"^(Enter|Tab|Escape|Space|Backspace|Delete|Up|Down|Left|Right|"
        r"PageUp|PageDown|Home|End|(?:Ctrl|Alt|Shift)\+\S+)(?:\s+(\d+))?\s*$"
    )

    for raw in Path(path).read_text().splitlines():
        line = raw.strip()
        if not line:
            continue
        if m := re.match(r"^# OVERLAY (.+?)\|(.+)$", line):
            pending = (m.group(1).strip(), m.group(2).strip())
            continue
        if line.startswith("#"):
            continue
        if m := re.match(r"^Set TypingSpeed (\d+)ms", line):
            type_speed = int(m.group(1)) / 1000
            continue
        if m := re.match(r"^Set Width (\d+)", line):
            width = int(m.group(1))
            continue
        if m := re.match(r"^Set Height (\d+)", line):
            height = int(m.group(1))
            continue
        if line == "Hide":
            shown = False
            continue
        if line == "Show":
            shown = True
            continue

        dur = 0.0
        is_press = False
        if m := re.match(r'^Type(?:@(\d+)ms)?\s+"(.*)"\s*$', line):
            speed = int(m.group(1)) / 1000 if m.group(1) else type_speed
            dur = len(m.group(2)) * speed
            is_press = True
        elif m := re.match(r"^Sleep\s+([\d.]+)(ms|s)?\s*$", line):
            dur = float(m.group(1)) / (1000 if m.group(2) == "ms" else 1)
        elif m := key_re.match(line):
            dur = (KEY_MS / 1000) * int(m.group(2) or 1)
            is_press = True

        if is_press and pending and shown:
            events.append((t, *pending))
            pending = None
        if shown:
            t += dur

    return events, (width, height)


def draw_keycap(key, label, out_path):
    """Website .fhint style: rounded keycap panel, amber label below."""
    key_font = ImageFont.truetype(MONO_BOLD, 54)
    label_font = ImageFont.truetype(MONO, 26)

    key_w = int(key_font.getlength(key))
    label_w = int(label_font.getlength(label))
    pad_x, pad_y = 42, 22
    panel_w, panel_h = key_w + pad_x * 2, 54 + pad_y * 2
    img_w = max(panel_w, label_w) + 24
    img_h = panel_h + 24 + 34

    img = Image.new("RGBA", (img_w, img_h), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    px = (img_w - panel_w) // 2
    # Bottom edge drawn first and offset, echoing the site's thicker
    # border-bottom keycap depth.
    d.rounded_rectangle((px, 4, px + panel_w, panel_h + 4), 16, fill=EDGE)
    d.rounded_rectangle((px, 0, px + panel_w, panel_h), 16, fill=PANEL,
                        outline=EDGE, width=2)
    d.text((img_w / 2, panel_h / 2 - 2), key, font=key_font, fill=FG,
           anchor="mm")
    d.text((img_w / 2, panel_h + 24 + 13), label, font=label_font,
           fill=AMBER, anchor="mm")
    img.save(out_path)


def build_filter(events, size, tmp):
    """ffmpeg inputs + filter_complex for scrim/keycap fade overlays."""
    scrim = tmp / "scrim.png"
    Image.new("RGBA", size, SCRIM).save(scrim)

    inputs, chains = [], []
    base = "[0:v]"
    for i, (t, key, label) in enumerate(events):
        cap = tmp / f"cap{i}.png"
        draw_keycap(key, label, cap)
        a, b = max(0.0, t - LEAD), t + TAIL
        stop = b + 0.5
        for j, png in enumerate((scrim, cap)):
            idx = 1 + i * 2 + j
            inputs += ["-loop", "1", "-t", f"{stop:.2f}", "-i", str(png)]
            pos = "0:0" if j == 0 else "(W-w)/2:(H-h)/2"
            chains.append(
                f"[{idx}:v]format=rgba,"
                f"fade=t=in:st={a:.2f}:d={FADE_IN}:alpha=1,"
                f"fade=t=out:st={b - FADE_OUT:.2f}:d={FADE_OUT}:alpha=1[o{idx}];"
                f"{base}[o{idx}]overlay={pos}:enable='between(t,{a:.2f},{b:.2f})'[v{idx}]"
            )
            base = f"[v{idx}]"
    return inputs, ";".join(chains), base.strip("[]")


def main():
    tape, raw, out_mp4, out_gif = sys.argv[1:5]
    events, size = parse_tape(tape)

    with tempfile.TemporaryDirectory() as td:
        tmp = Path(td)
        if events:
            inputs, fc, last = build_filter(events, size, tmp)
            for t, key, _ in events:
                print(f"  overlay {key!r} at {t:.2f}s")
            run = ["ffmpeg", "-y", "-loglevel", "error", "-i", raw, *inputs,
                   "-filter_complex", fc, "-map", f"[{last}]"]
        else:
            run = ["ffmpeg", "-y", "-loglevel", "error", "-i", raw]
        subprocess.run([*run, "-c:v", "libx264", "-pix_fmt", "yuv420p",
                        "-crf", "20", "-movflags", "+faststart", out_mp4],
                       check=True)

    subprocess.run(
        ["ffmpeg", "-y", "-loglevel", "error", "-i", out_mp4, "-vf",
         "fps=25,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];"
         "[b][p]paletteuse=dither=bayer:bayer_scale=4",
         out_gif],
        check=True)
    subprocess.run(["gifsicle", "-O3", "--lossy=40", "-b", out_gif],
                   check=True)


if __name__ == "__main__":
    main()
