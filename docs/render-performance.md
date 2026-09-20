# Rendering performance

The primary outcome is **time from incoming output to a terminal write containing
the newest line**. CPU time per frame helps explain that outcome, but is not a
substitute for it. Every optimization must preserve the rendered cells and the
ordering of output, prompts, input, and resize events.

## Run and compare

Use Go 1.26.5 or the version in `go.mod`, and Python 3 (standard library only).
Run from the repository root, on the same otherwise-idle machine and power mode:

```sh
make bench-render
python3 tools/render-perf.py record perf-results/before
# Make a change.
python3 tools/render-perf.py record perf-results/after
python3 tools/render-perf.py compare perf-results/before perf-results/after
```

`make bench-render` runs the UI race tests and executes each workload once to
check that the harness works. Its timings are **not** performance conclusions.
`record` runs the race tests separately, then five uninstrumented measurements
per workload. It saves raw Go benchmark output, revision, working-tree diff,
source hashes, Go/environment settings, and test output. Existing result
directories are never overwritten. Artifacts under `perf-results/` are ignored
by Git. `--skip-tests` is appropriate only after this revision's UI tests passed.

Keep the matched raw runs until the change is reviewed. Afterward, record the
decision and essential measurements here, then delete the generated run
directories, test binaries, profiles, and logs. They are disposable investigation
artifacts, not permanent project documentation.

For the primary output-latency workloads, use longer measurements:

```sh
python3 tools/render-perf.py record perf-results/output-before \
  --bench '^BenchmarkOutputLatency$' --time 10s --count 5
```

Read `p50-ns`, `p95-ns`, and `p99-ns` together with `samples`. A one-second
measurement of a slow burst can contain only one or two observations; its p99
is then just the largest observation. Use at least hundreds of observations
before making claims about tail latency. `compare` reports medians and ranges
across runs, including latency percentiles, allocations, and emitted bytes. It
does not claim statistical significance or enforce a machine-dependent time
threshold. Raw `bench.txt` files are also usable with `benchstat` if installed.
Missing workloads make a comparison fail instead of silently disappearing.
The reader recognizes the former `RenderCompose` name as `RenderScreen`; only
the name changed, so older baseline files remain comparable.
Check `metadata.json` before comparing different machines, toolchains, settings,
or benchmark definitions. Changing a fixture requires a new baseline.

## What each workload proves

| Workload | One operation | Purpose |
|---|---|---|
| `OutputLatency/idle` | One line after the model's pending frame work has settled | First-output delay; settling is excluded from timing |
| `OutputLatency/single` | One line, waiting for its terminal write before sending the next | Continuous small updates |
| `OutputLatency/burst100`, `burst1000` | Enqueue a burst and wait for its last line's terminal write | Backlog drain and newest-output latency |
| `OutputLatency/draft100_burst100` | Same 100-line burst while a multiline draft is open | Unrelated draft editing work delaying MUD output |
| `OutputLatency/auto_side_draft1000_burst100`, `beside_pane_draft1000_burst100` | 100-line burst with a 1,000-line draft and an auto-sized pane or adjacent pane | Verify layout savings reach the terminal writer |
| `RenderAutoLayoutOutput` | One incoming line in the default layout, with an auto-sized chat pane, or with input beside a pane in an auto-sized row | Per-message layout costs with 0/1,000 draft lines; rendering is deferred |
| `RenderDraftCursor` | Left then Right in a 1,000-line draft, processing both updates and consuming their Session events | Draft navigation and layout work; rendering is deferred |
| `RenderUpdate` | One line inside an open render-throttle window | Per-message work, with 0/100/1,000 draft lines |
| `RenderFlood` | 100 lines, ten prompt updates, one explicit frame boundary | Fixed-work throughput, independent of timer scheduling |
| `RenderScreen` | Render an unchanged two-pane scene at 80×24 or 270×66 | Full rendering cost with warm widget caches |
| `RenderLayout` | Resolve geometry for a populated scene | Geometry and border-planning allocations |
| `RenderPipeline` | Update, render, decode, diff, and encode a changing frame | Renderer CPU cost and `terminal-B/op`, without timer delays |
| `RenderUnchanged`, `RenderResize` | Repeated unchanged prompt, or alternating terminal sizes | No-op overhead and resizing |
| `RenderSearch` | Search 100,000 rows for a common or missing term | UI-thread stalls, with plain and colored rows |
| `RenderPane` | Render 24/66/200 visible chat rows | Wrapping and row-copy scaling |

The latency harness uses production `BubbleTeaUI.Print` and its FIFO, the real
Model, the 16 ms rendering throttle, and the real Bubble Tea renderer and its
default frame clock. A test-only wrapper adds a sequence marker to the window
title **only when that output has been rendered**. The terminal writer observes
the marker in the same flush as the screen. This avoids falsely acknowledging
a call to `View` or expecting whole line strings in terminal diff output.
Bursts alternate their content so the renderer cannot treat successive complete
bursts as unchanged screens.

The probe adds a small title/observer overhead. It measures UI enqueue to final
writer delivery, **not TCP receipt, Lua trigger execution, terminal emulator
display time, or pixels on a physical display**. It runs with a live output window.
It is a closed-loop burst workload, not a prediction of maximum sustainable
network throughput. During a burst, intermediate frames can appear before the
reported final-line latency. Memory goes to a discard/counting observer, not an
ever-growing captured terminal log. The deterministic CPU workloads deliberately
control frame boundaries instead of waiting for real timers.

## Profile the cost separately

```sh
python3 tools/render-perf.py profile perf-results/flood-profile
python3 tools/render-perf.py profile perf-results/draft-profile \
  --bench '^BenchmarkRenderUpdate$/^draft_lines=1000$'
go tool pprof -http=localhost:8080 \
  perf-results/flood-profile/bench.test perf-results/flood-profile/cpu.pprof
```

Use an exact benchmark selector for a focused profile. `profile` saves the test
binary, CPU and heap profiles, and text summaries. The heap summary uses
`alloc_space`, which shows allocation churn rather than only retained objects.
Profiled measurements must not be used as before/after timing samples. CPU
profiles do not explain time spent waiting for frame clocks; use latency metrics
for that. Both the live latency observer and the model-side marker have focused
correctness tests; to exercise their concurrency under the race detector:

```sh
go test -race ./ui/tui -run '^$' -bench '^BenchmarkOutputLatency$' -benchtime=1x
```

## Preserve what users see

The normal UI tests cover wrapping, border junctions, resizing, search anchors,
prompt commits, ring eviction, and input behavior. `TestRenderSnapshots` adds
five deterministic scenes using the same fixture as the benchmarks: normal,
draft editor, picker, search, and scrolled output receiving new text. They
include colors, wide characters, combining marks, emoji, and tabs. Snapshots
normalize ANSI through terminal cells before comparison, so equivalent escape
encodings can pass. Quoted rows preserve styles and padding in reviewable files.

For an intentional visual change only:

```sh
go test ./ui/tui -run '^TestRenderSnapshots$' -update-render
git diff -- ui/tui/testdata/render
```

Review that diff; do not regenerate snapshots merely to make a failing
optimization pass. They capture the current appearance, not proof that every
current behavior is ideal. Existing assertions remain necessary. Actual terminal
checks for key negotiation or display artifacts follow `docs/testing.md`.

## Optimization order

1. Eliminate geometry and draft editor work from ordinary output updates
   when their sizes cannot change. Preserve append-time wrapping and apply any
   pending resize before appending the next line.
2. Reuse resolved geometry and border cells until geometry actually changes.
   Avoid building a terminal-sized border grid on every message.
3. Measure frame scheduling against the latency workloads. Rune and Bubble Tea
   have separate frame clocks; removing Rune's throttle without replacing its
   work coalescing would restore per-message full rendering.
4. Reduce repeated ANSI/cell conversion and whole-screen clearing. Verify both
   CPU and emitted bytes, plus the rendered-cell snapshots.
5. Bound search work and avoid rewrapping unchanged sidebar content where those
   features delay incoming output.

Prefer removing work and ownership layers before introducing another cache or
renderer. A change should identify which dependency invalidates geometry, text,
or rendering, and should have a benchmark that would reveal its regression.


## Reviewed results

These are historical measurements on an AMD Ryzen 7 4800U, Linux/amd64,
Go `1.27.1-X:nodwarf5`, for the specific revisions below; they are not a universal
latency guarantee. The initial simplification compared `8eaa1a6` with `2776f35`
(five one-second runs); the draft/layout follow-up compared `2696987` with
`da88a67` (five 500ms runs with matching benchmark fixtures).

| Change and incoming-output workload | Before mean | After mean |
|---|---:|---:|
| Initial simplification: 1,000-line burst | 109.52 ms | 39.81 ms |
| Initial simplification: 100-line burst with 100-line draft | 1,312.58 ms | 46.55 ms |
| Follow-up: 100-line burst, 1,000-line draft beside a pane | 2,829.97 ms | 44.32 ms |

Values are medians of run means, from UI enqueue to the terminal writer.
The slow draft cases had only one burst per baseline run; these comparisons
support mean improvements, not tail-latency claims. Gains came from avoiding
layout and draft shaping on ordinary output updates. Idle-line delivery remained
around 20 ms. Ordinary render/resize workloads showed no uniform speedup.
Current ownership and invalidation rules are documented in [Architecture](architecture.md).

A direct-cell handoff experiment against `00a0d8f` was **rejected**. It patched
Bubble Tea v2.0.9 to accept immutable cell snapshots, retaining both frame clocks.
Ten one-second pipeline runs per version at 270×66 and five ten-second runs for
each of two writer-latency cases produced these medians:

| Measurement | Existing renderer | Snapshot handoff |
|---|---:|---:|
| Prompt pipeline time | 4.96 ms | 8.24 ms |
| Scroll pipeline time | 5.28 ms | 8.17 ms |
| Allocated bytes per scroll frame | 214 KB | 2,222 KB |
| Single-line delivery mean | 22.28 ms | 19.90 ms |
| 1,000-line burst delivery mean | 36.70 ms | 33.27 ms |

Delivery p95 improved by only about 0.4 ms. Snapshot copying accounted for 97%
of sampled allocated bytes. UI race tests and existing goldens passed, but the
real renderer comparison found different joined-emoji widths in legacy terminal
width mode. The memory/time regressions, width mismatch, and unsupported API
outweighed the modest delivery gains. No production handoff change was accepted.
A reusable buffer handoff remains unmeasured and needs an explicit ownership and
width contract; this result does not justify maintaining a private renderer fork.
