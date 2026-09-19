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
Check `metadata.json` before comparing different machines, toolchains, settings,
or benchmark definitions. Changing a fixture requires a new baseline.

## What each workload proves

| Workload | One operation | Purpose |
|---|---|---|
| `OutputLatency/idle` | One line after the model's pending frame work has settled | First-output delay; settling is excluded from timing |
| `OutputLatency/single` | One line, waiting for its terminal write before sending the next | Continuous small updates |
| `OutputLatency/burst100`, `burst1000` | Enqueue a burst and wait for its last line's terminal write | Backlog drain and newest-output latency |
| `OutputLatency/draft100_burst100` | Same 100-line burst while a multiline draft is open | Unrelated editor work delaying MUD output |
| `RenderUpdate` | One line inside an open paint-throttle window | Per-message work, with 0/100/1,000 draft lines |
| `RenderFlood` | 100 lines, ten prompt updates, one explicit frame boundary | Fixed-work throughput, independent of timer scheduling |
| `RenderCompose` | Paint an unchanged two-pane scene at 80×24 or 270×66 | Full composition cost with warm widget caches |
| `RenderLayout` | Resolve geometry for a populated scene | Geometry and border-planning allocations |
| `RenderPipeline` | Update, compose, decode, diff, and encode a changing frame | Renderer CPU cost and `terminal-B/op`, without timer delays |
| `RenderUnchanged`, `RenderResize` | Repeated unchanged prompt, or alternating terminal sizes | No-op overhead and resizing |
| `RenderSearch` | Search 100,000 rows for a common or missing term | UI-thread stalls, with plain and colored rows |
| `RenderPane` | Render 24/66/200 visible chat rows | Wrapping and row-copy scaling |

The latency harness uses production `BubbleTeaUI.Print` and its FIFO, the real
Model, the 16 ms composition throttle, and the real Bubble Tea renderer and its
default frame clock. A test-only wrapper adds a sequence marker to the window
title **only when that output has been composed**. The terminal writer observes
the marker in the same flush as the screen. This avoids falsely acknowledging
a call to `View` or expecting whole line strings in terminal diff output.
Bursts alternate their content so the renderer cannot treat successive complete
bursts as unchanged screens.

The probe adds a small title/observer overhead. It measures UI enqueue to final
writer delivery, **not TCP receipt, Lua trigger execution, terminal emulator
painting, or pixels on a physical display**. It runs with a live output viewport.
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

The normal UI tests cover wrapping, frame junctions, resizing, search anchors,
prompt commits, ring eviction, and input behavior. `TestRenderSnapshots` adds
five deterministic scenes using the same fixture as the benchmarks: normal,
multiline editor, picker, search, and scrolled output receiving new text. They
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

1. Eliminate geometry and multiline-editor work from ordinary output updates
   when their sizes cannot change. Preserve append-time wrapping and apply any
   pending resize before appending the next line.
2. Reuse resolved geometry and border cells until geometry actually changes.
   Avoid building a terminal-sized border grid on every message.
3. Measure frame scheduling against the latency workloads. Rune and Bubble Tea
   have separate frame clocks; removing Rune's throttle without replacing its
   work coalescing would restore per-message full composition.
4. Reduce repeated ANSI/cell conversion and whole-screen clearing. Verify both
   CPU and emitted bytes, plus the rendered-cell snapshots.
5. Bound search work and avoid rewrapping unchanged sidebar content where those
   features delay incoming output.

Prefer removing work and ownership layers before introducing another cache or
renderer. A change should identify which dependency invalidates geometry, text,
or paint, and should have a benchmark that would reveal its regression.
