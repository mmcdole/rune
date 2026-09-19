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
appends reuse layout; auto-sized panes and containers still trigger measurement.
Resize/layout messages apply geometry immediately, preserving ordering and
append-time wrapping. The editor reuses its wrapped rows until text, cursor,
or width changes. A layout's first render resolves border junctions into
positioned cells; later renders reuse them. Widgets clear their own rectangles;
a full canvas clear happens on the first render after layout changes. Identical bars/prompts schedule no work.
Side-pane rows are filled into one bounded slice instead of repeatedly prepended.

## Vocabulary and ownership

| Before | Now | What earned its place |
|---|---|---|
| `outputController` → `Viewport` → `ScrollbackBuffer` | `Output` → `Scrollback` | One widget owns wrapping, prompt, scrolling, and the visible window; one ring stores retained rows |
| `transcript`, `history`, `buffer` as names for output storage | `scrollback` | The retained rows, not another rendering layer; command history remains a separate feature |
| `compose`, `paint`, compositor | `render` | One name for building the screen, including `renderTick`, `renderInterval`, and `render.go` |
| multiline `composer`, `modeCompose` | `editor`, `modeEditor` | Text editing has its own name and files |
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

The production TUI goes from 32 files / 6,335 lines to 30 files / 6,217 lines
(118 fewer lines, excluding tests, benchmarks, and docs). Most changed lines
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
on this branch, not dismissed as timing noise. Raw control runs and revision/order
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

## Validation

- Full default-backend race/shuffle suite (`make test`) passed; UI race tests
  also passed after the final deferred-border change.
- `make check` passed with `GOTOOLCHAIN=go1.26.5`, the repository's declared Go
  version. The host Go 1.27 export format is newer than pinned staticcheck supports.
- The live Bubble Tea latency workloads passed a separate `-race -benchtime=1x`
  smoke run; those instrumented timings are excluded from the tables.
- All five rendered-cell snapshots match unchanged. The multiline scene's file
  was renamed from `composer.golden` to `editor.golden` without changing its bytes.
- New regressions cover unchanged-draft appends, nested auto-sized panes, no-op
  updates, removed-pane cleanup, one-row prompts, and editor cache invalidation.
- Isolated tmux smoke checks passed output, command dispatch, and terminal resize
  without a MUD connection or changes to the user's config.
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
  lines each render. Editing or moving the cursor in a large draft still shapes
  the whole draft; incoming output no longer does. These costs remain measurable.

The harness measures UI enqueue through the real FIFO and Bubble Tea terminal
writer, including both clocks. It excludes network receipt, Lua triggers, and
the terminal emulator's physical display time. This is evidence about Rune's
UI latency, not a claim about complete network-to-pixel latency.
