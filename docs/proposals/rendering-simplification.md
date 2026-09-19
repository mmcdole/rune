# Rendering simplification and measurements

Branch: `simplify-rendering`. Production baseline: `0307cda` (latest `main`
when work began). Harness-only baseline commit: `8eaa1a6`. The priority is new
MUD text reaching the terminal promptly, with fewer owners and fewer terms.

## What the branch changes

The old incoming-line path rebuilt the entire layout, repeatedly wrapped any
open multiline draft, allocated terminal-sized border grids, and then checked
whether screen rendering was due. A 100-line draft made a 100-line incoming
burst take about 1.31 seconds to reach the terminal writer. Throttling the final
render did not throttle that per-message work.

The new path is:

```text
incoming line → Output.Write → mark screen dirty
                              ↓ when rendering is due
                render retained layout → Bubble Tea → terminal writer
```

`Model.dispatch` reports whether pixels and geometry may change. Ordinary
appends reuse layout; writes to panes with auto sizing (including an auto-sized
ancestor) trigger measurement. The plan records those pane names.
Resize/layout messages apply geometry immediately, preserving ordering and
append-time wrapping. The draft editor reuses its wrapped rows until text
or width changes; cursor movement locates an existing insertion point. A layout's first render resolves border junctions into
positioned cells; later renders reuse them. Widgets clear their own rectangles;
a full canvas clear happens on the first render after layout changes. Identical bars/prompts schedule no work.
Side-pane rows are filled into one bounded slice instead of repeatedly prepended.

## Vocabulary and ownership

| Before | Now | What earned its place |
|---|---|---|
| `outputController` → `Viewport` → `ScrollbackBuffer` | `Output` → `Scrollback` | One widget owns wrapping, prompt, scrolling, and the visible window; one ring stores retained rows |
| `transcript`, `history`, `buffer` as names for output storage | `scrollback` | The retained rows, not another rendering layer; command history remains a separate feature |
| `compose`, `paint`, compositor | `render` | One name for building the screen, including `renderTick`, `renderInterval`, and `render.go` |
| multiline `composer`, `modeCompose` | `draftEditor`, `modeDraftEditor` | Names the in-TUI editor explicitly; plain editor actions still mean `$EDITOR` |
| `inputController.editorAction` | `inputAction` | Resolves actions shared by all input modes |
| `paneResource`, `paneRegistry`/`paneStore` | `pane`, model's `panes` map | Removed the registry type and its forwarding methods; one lookup/create helper preserves output identity |
| layout `surface` | `widget` | It already implements `widget.Widget` |
| private `layoutNode` | `resolvedNode` | Distinguishes measured/placed nodes from the declarative `ui.LayoutNode` |
| `frameGrid`, `frameEdges`, `frames.go` | `borderGrid`, `borderEdges`, `borders.go` | Borders are not screen frames |
| `layout_size.go` | `layout_measure.go` | Measurement/allocation is distinct from placement |
| `util.VisibleLen` | `ansi.StringWidth` | Removed a forwarding name that obscured terminal columns |
| `widget/text.go` | `util.ClipRow` beside ANSI helpers | Removed a one-helper file |
| stored scroll mode plus offset | mode derived from offset | One source of truth for live versus scrolled output |

Output and side panes remain distinct because their behavior differs: output
retains physical rows as they arrived, while side panes reflow logical lines.
Combining them would introduce policy flags and change scrolling units. Layout
and rendering also remain distinct because content changes much more often than
rectangles. The small Widget interface and input controller retain real duties:
measurement/placement, and submission/picker/search event ordering respectively.

The first implementation (`2776f35`) reduced production TUI code from 32 files /
6,335 lines to 30 files / 6,217 lines (118 fewer lines, excluding tests,
benchmarks, and docs). Most changed lines
are consistent renames; the ownership and update-path changes are the point.

## Measurements

Baseline `8eaa1a6` versus implementation `2776f35`, measured sequentially on the
same AMD Ryzen 7 4800U machine, Linux/amd64, Go `1.27.1-X:nodwarf5`, default
16-way scheduler. Five uninstrumented runs per workload, `-benchtime=1s`.
Tables report the median of each run's mean `ns/op`; ranges show the smallest
and largest run means, not confidence intervals. These runs do not establish
p99 latency: the old large-draft case has one burst per run (five total),
versus 24–26 bursts per run after the change.

| Incoming output to terminal writer | Before mean (run range) | After mean (run range) | Change |
|---|---:|---:|---:|
| One line after idle | 20.47 ms (20.34–21.74) | 19.69 ms (18.12–20.53) | -3.8% |
| Continuous single lines | 24.50 ms (24.07–24.94) | 22.65 ms (22.45–22.97) | -7.6% |
| 100-line burst | 43.33 ms (42.62–46.75) | 37.05 ms (35.32–40.52) | -14.5% |
| 1,000-line burst | 109.52 ms (104.47–117.58) | 39.81 ms (36.87–41.82) | -63.7% |
| 100-line burst with 100-line draft | 1312.58 ms (1261.30–1343.09) | 46.55 ms (43.40–49.92) | -96.5% |

The large-draft burst is **28.2× faster** in this measurement; the 1,000-line
burst is **2.75× faster**. Idle single-line delay remains around 20ms.

| Supporting workload | Before | After |
|---|---:|---:|
| Short-line update, empty draft | 17.60 µs | 50.1 ns |
| Short-line update, 100-line draft | 9.12 ms | 50.1 ns |
| Short-line update, 1,000-line draft | 90.10 ms | 49.4 ns |
| 100 lines + 10 prompts + one render | 10.28 ms | 2.19 ms |
| Render warm 80×24 screen | 426.86 µs | 454.81 µs |
| Render warm 270×66 screen | 2.49 ms | 2.23 ms |
| Rebuild two-pane layout | 21.16 µs | 21.29 µs |
| Prompt update through decode/diff/encode | 5.52 ms | 5.01 ms |
| Scroll update through decode/diff/encode | 5.60 ms | 5.14 ms |
| Unchanged prompt + closing tick | 3.12 ms | 50.6 ns |
| Alternating terminal sizes | 1.46 ms | 1.45 ms |
| Render 200 side-pane rows | 203.79 µs | 19.41 µs |
| Missing query in 100,000 plain rows | 7.67 ms | 8.76 ms |
| Missing query in 100,000 colored rows | 69.83 ms | 69.86 ms |

**Allocated bytes, not retained memory:** the fixed 100-line flood falls from
4.758 MB to 0.197 MB per operation (**95.9% less**).
Short-line updates allocate **zero bytes and zero objects** for all three draft
sizes; the old 1,000-line draft allocated 24.9 MB per incoming line. The
200-row side pane falls from 355 KB to 16 KB.

**Tradeoffs and unchanged costs:** layout rebuild time/allocations are effectively
unchanged. Resize time is also similar, but allocations rise from 454 KB to
467 KB (3%) for the cached positioned borders. The small-screen render median
is 6.5% slower with overlapping run ranges (before 424–474 µs, after 403–493 µs);
this is not evidence of a uniform render-speed gain. Plain search misses were
14.3% slower in the full run despite no search-algorithm change. An alternating
three-run control reproduced it: 7.67 ms before versus 8.68 ms after (+13.1%),
with unchanged allocations. Its cause is not isolated; this is a known regression
in the first implementation, not dismissed as timing noise. The follow-up
measurements below are separate observations. Raw control runs and revision/order
metadata are in `perf-results/search-control`.

A separate output-flood profile now spends about **54% of sampled CPU beneath
`ultraviolet.renderLine`** and **84% of allocated bytes beneath
`ultraviolet.(*Buffer).Render`**. String serialization is the next major CPU/
allocation target. These are cumulative profile shares, not timing samples; the
profile does not include Bubble Tea’s terminal flush or its waiting time.

See [the harness](../render-performance.md) for workload definitions and
measurement boundaries. Reproduce the comparison from the repository root:

```sh
# With the harness-only baseline (8eaa1a6) checked out:
python3 tools/render-perf.py record perf-results/before-simplification
# With the implementation (2776f35) checked out:
python3 tools/render-perf.py record perf-results/final-simplification
python3 tools/render-perf.py compare \
  perf-results/before-simplification perf-results/final-simplification
```

Local ignored artifacts include each directory's `bench.txt`, `metadata.json`,
and `changes.patch`; profiles are saved separately in
`perf-results/final-flood-profile`. The old `RenderCompose` workload has been
renamed `RenderScreen`; the comparison tool recognizes that name change.

## Review follow-up: simplicity versus gains

The follow-up keeps the existing owners and adds no production files or types.
Production TUI code is now 30 files / 6,250 lines: 33 more lines than the first
implementation, still 85 fewer lines and two fewer files than the original.

- `layoutPlan.autoPanes` records the pane names whose text can affect geometry.
  A write only requests layout if its pane is in that set. This covers both
  auto-sized leaves and auto-sized ancestors, without a dependency graph.
- `needsClear` means layout changed and the next render must erase uncovered
  cells. Border preparation no longer doubles as the canvas-clear signal.
- `draftEditor` keeps one shaped width. Cursor lookup reuses its rune insertion
  points. Height measurement returns immediately for eight physical lines;
  shorter drafts use the same shaper bounded to the eight-row height cap,
  without evicting the editing layout. Line counts are cached until a text edit.
- `ClipRow` uses byte length as a cheap upper bound before measuring cells.
- `draftEditor` and `inputAction` replace ambiguous internal editor names.
  Current documentation uses "draft editor"; `$EDITOR`, `input.open_editor`, and
  the footer's "editor" hint keep their established meanings. The old website
  section anchor remains as a compatibility alias for incoming links.

Per-widget dirty tracking and additional pane caches are deferred. They add
invalidation rules, while the existing profile attributes most allocation to
whole-screen string serialization. Changed draft text still requires shaping;
this change does not introduce incremental editing or a multi-width cache.

The follow-up benchmarks add auto-pane layouts, draft navigation, and actual
terminal-output delivery in those layouts. They use five runs of 500ms on the
same machine/toolchain as above, sequentially before and after. The slow
adjacent-pane baseline contains only one burst per run, so it supports a mean
latency comparison, not a p99 claim. Cursor workloads consume their Session
events each iteration to avoid measuring queue-overflow warnings.

Baseline: `2696987` with the benchmark-only changes saved in
`perf-results/before-render-followup-v2/changes.patch`. Implementation: `da88a67`.
The benchmark functions and fixtures match on both sides. Table entries are
medians of run means, with min–max ranges where shown.

| Terminal-writer latency | Before | After |
|---|---:|---:|
| One line after idle | 21.32 ms | 20.41 ms |
| Continuous single lines | 22.88 ms | 22.54 ms |
| 100-line burst | 37.86 ms | 35.45 ms |
| 1,000-line burst | 42.65 ms | 41.39 ms |
| 100-line burst with 100-line draft, ordinary layout | 47.00 ms (41.36–49.78) | 49.57 ms (45.09–49.73) |
| 100-line burst, 1,000-line draft, separate auto-sized chat pane | 64.42 ms (64.13–66.42) | 42.45 ms (36.45–49.12) |
| 100-line burst, 1,000-line draft beside a pane in an auto-sized row | 2,829.97 ms (2,628.20–3,429.02) | 44.32 ms (40.28–49.38) |

The adjacent-pane case is **63.8× faster** to the terminal writer. Allocated
bytes per burst fall from **1.224 GB to 348 KB**. These are allocation totals,
not retained memory. The separate auto-pane case improves by **34.1%**.

| Supporting work | Before | After |
|---|---:|---:|
| Output update, auto chat pane and 1,000-line draft | 140.04 µs / 59 KB | 77.9 ns / 0 B |
| Output update, draft beside pane | 26.49 ms / 8.37 MB | 76.4 ns / 0 B |
| Left + Right, 1,000-line draft, default layout | 28.82 ms / 8.59 MB | 1.58 ms / 282 KB |
| Left + Right, draft beside pane | 56.16 ms / 16.90 MB | 1.70 ms / 278 KB |
| Warm render, 80×24 | 435 µs | 451 µs |
| Warm render, 270×66 | 2.45 ms | 2.47 ms |
| Fixed flood, including one render | 2.52 ms | 2.58 ms |
| Default-layout short-line update | 51.3 ns | 54.6 ns |
| Missing search in 100,000 plain rows | 8.84 ms | 7.81 ms |
| Missing search in 100,000 colored rows | 72.02 ms | 64.63 ms |

The update and cursor rows defer rendering; they are CPU/allocation work, not
terminal-output latency. Cursor navigation still copies draft strings for
Session notifications and rebuilds layout; it is cheaper, not constant-time.
Ordinary output/render workloads show no uniform gain: the 100-line-draft
latency median is 5.5% higher, with overlapping ranges; warm rendering and
fixed-flood ranges also overlap. Default line updates cost about 3–5 ns more,
with zero allocations. Rendering 66 side-pane rows is 3.6% slower (6.65 to
6.89 µs) in these runs. Search is faster in this run, but no search algorithm
changed, and the earlier regression's cause remains unisolated. Neither the
small clipping guard nor the clearer names get a standalone speedup claim.

To reproduce the local comparison, use the saved benchmark patch on the
baseline and run the same command in each checkout with fresh result directories:

```sh
# Baseline checkout at 2696987: apply the absolute path to
# perf-results/before-render-followup-v2/changes.patch first.
# Implementation checkout at da88a67: no benchmark patch needed.
python3 tools/render-perf.py record perf-results/repeat --count 5 --time 500ms

# The completed measurements in this workspace:
python3 tools/render-perf.py compare \
  perf-results/before-render-followup-v2 perf-results/after-render-followup
```

The complete comparison is `perf-results/render-followup-comparison.txt`.

## Validation

- Full default-backend race/shuffle suite (`make test`) passed; UI race tests
  also passed after the final draft-layout change.
- `make check` passed with `GOTOOLCHAIN=go1.26.5`, the repository's declared Go
  version. The host Go 1.27 export format is newer than pinned staticcheck supports.
- All seven live Bubble Tea latency workloads passed a separate
  `-race -benchtime=1x` smoke run; those instrumented timings are excluded
  from the tables.
- All five rendered-cell snapshots match unchanged. The multiline scene's file
  was renamed from `composer.golden` to `editor.golden` in the first pass, then
  to `draft_editor.golden` in the follow-up, without changing its bytes.
- Regression tests cover per-pane layout decisions, nested auto-sized panes,
  no-op updates, uncovered-cell clearing after border preparation, one-row
  prompts, cursor reuse at tab and Unicode insertion points, text-edit
  invalidation, and bounded measurement against full shaping over fixed-seed
  inputs and widths 1–80.
- Isolated tmux checks passed output, command dispatch, structured paste,
  cursor movement, resize, and discard without a MUD connection or changes
  to the user's config. Captures are in `perf-results/terminal-followup`.
- The website builds with the renamed sections and updated internal links.
- LuaJIT's full suite could not link: this host lacks `libluajit-5.1.a`. Tagged
  vet passed; the LuaJIT runtime suite remains unverified here.

## Remaining bottlenecks

- **Two clocks:** Rune coalesces renders at 16ms and Bubble Tea flushes at 60Hz.
  This branch preserves both. Removing one safely is a separate experiment;
  deleting Rune's throttle alone restores expensive per-message rendering.
- **Two ANSI/cell conversions:** widget strings become Rune cells and strings,
  then Bubble Tea decodes those strings again. Full-screen rendering is still
  several milliseconds at 270×66. A direct cell handoff needs an API decision;
  this branch does not introduce a custom terminal engine.
- **Search:** a missing query still scans up to 100,000 rows synchronously and
  strips colored text again. That can stall incoming output. Bounded search
  work and/or cached plain text need their own measured change.
- **Side-pane wrapping and changed drafts:** side panes still rewrap visible
  lines each render. Text edits and actual width changes still shape the whole
  draft. Cursor movement and incoming output reuse its rows. Small drafts
  measured at a different width use the same shaper capped at eight rows.

The harness measures UI enqueue through the real FIFO and Bubble Tea terminal
writer, including both clocks. It excludes network receipt, Lua triggers, and
the terminal emulator's physical display time. This is evidence about Rune's
UI latency, not a claim about complete network-to-pixel latency.
