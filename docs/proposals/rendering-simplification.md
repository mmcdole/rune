# Rendering review: speed through less work

Reviewed production code at `0307cda`. The accompanying changes add measurement
and correctness tooling; they do not change runtime rendering. The priority is
how quickly new MUD output reaches the terminal, alongside fewer concepts and
clearer ownership.

## Findings, in priority order

### 1. Incoming output repeatedly lays out an unrelated multiline draft

[`Model.Update`](../../ui/tui/model.go) unconditionally calls `applyLayout`.
Input resolution and preferred-height calculation call `MeasureHeight`, which
builds the entire composer layout. `Input.SetSize` builds it again even when
dimensions did not change. Frame-edge discovery also repeatedly calls `Rules`,
which constructs labels and copies/counts the draft text. Painting builds the
composer layout again.

This is the clearest user-visible bottleneck. The new real-renderer benchmark
measures about **1.3 seconds** to display the end of a 100-line incoming burst
with a 100-line draft open, versus about **45 ms** without that draft. An isolated
throttled output update with a 1,000-line draft costs about **89 ms and 25 MB**.
These are local measurements, not universal thresholds; short slow-case runs
do not establish p99 latency.

**Change:** ordinary appends should not remeasure an unchanged editor. Compute
draft wrapping from text revision and width; use its existing insertion points
to locate the cursor. Keep cursor visibility adjustment separate from geometry
measurement. A size-equality early return in `SetSize` alone is insufficient:
editing or moving the cursor still needs to keep the caret visible.

### 2. Paint is throttled; geometry and border allocation are not

[`resolveLayout`](../../ui/tui/layout.go) rebuilds the active tree and creates
two screen-sized Boolean grids after every message, including ordinary output,
unchanged prompt updates, ignored input, and closing throttle ticks.
[`newFrameGrid`](../../ui/tui/frames.go) accounts for **about 89% of allocated
bytes** in the original output-flood allocation profile. At 270×66, a simple
output update allocates about **39 KB before painting**. Appending a short line
to the transcript alone was approximately **0.1 microseconds**, so the ring
buffer is not the place to start.

**Change:** retain the resolved layout until a dependency that can alter
geometry changes. Distinguish geometry changes from paint changes. Ordinary
output in a fixed/fractional pane generally changes paint only; an auto-sized
pane, bar appearing/disappearing, input wrapping, visibility, or terminal resize
can change geometry. Preserve message ordering: a resize or layout change must
take effect before subsequent output uses its append-time wrapping width.

Reuse the border geometry with that layout. Resolve junction glyphs once into
a compact list of positioned cells. This removes both screen-sized allocations
per message and the whole-grid scan plus neighbor lookup on every paint. Keep
dynamic labels separate; a changing draft count or pane scroll title must not
require rebuilding border connectivity.

### 3. Two frame clocks contribute to visible latency

Rune limits composition with a 16 ms on-demand `composeTick`. The pinned Bubble
Tea renderer flushes on its own default 60 Hz clock. A line can wait for a Rune
composition and then for a terminal flush. The local harness sees single-line
delivery around **20–25 ms per operation**, and continuous 100-line bursts around
**45 ms**. Composing the 270×66 two-pane scene alone costs about **2.5 ms**.

**Change:** evaluate one owner of frame scheduling after fixing per-message
work. Preserve burst coalescing and immediate state updates. Simply deleting
Rune's throttle would make full composition run after every Bubble Tea message.
The latency benchmark uses the real clocks specifically to catch that tradeoff.
It measures writer delivery, not the terminal emulator's physical paint time.

### 4. The screen crosses the ANSI/cell boundary twice

[`compose`](../../ui/tui/render.go) takes widget ANSI strings, parses them into
Ultraviolet cells, then serializes the entire canvas back into ANSI. Bubble Tea
parses that ANSI into another cell grid before diffing and emitting terminal
output. The pinned `StyledString.Draw` clears its rectangle first; Rune also
clears the whole canvas. Unchanged widget text is still parsed on each paint.

**Change:** choose one terminal representation at the composition boundary.
Initially, reuse parsed visible rows or unchanged widget regions and avoid
redundant clearing. A direct cell handoff would remove an entire conversion,
but Bubble Tea's current `View.Content` boundary is string-based: this needs an
upstream/API decision, not an assumption that Rune can switch a method call.
Keep terminal diffing, Unicode handling, editor suspension, and terminal modes
working. A separate custom terminal engine is not justified by this review.

### 5. Search and side panes can stall the same UI loop

[`Search.rescan`](../../ui/tui/widget/search.go) caps the number of *matches*,
not the number of rows examined. A missing query can synchronously scan all
100,000 rows and strip ANSI again on every edit. The initial probe measured
about **62 ms and 6.4 MB** for a miss over colored history. During that work,
new MUD output waits.

**Change:** reuse searchable plain text for immutable rows if the memory
tradeoff is worthwhile, then bound scanning work per UI turn. Incremental scans
can stay on the owning goroutine and cancel by query revision; introducing
shared mutable buffers and worker synchronization is unnecessary initially.
Cached stripping alone will not bound a scan of rare or missing matches.

[`Pane.ContentRows`](../../ui/tui/widget/pane.go) repeatedly prepends wrapped
rows to an expanding slice, causing quadratic copying in the visible row count.
It rewraps unchanged text on each paint. Fill a preallocated window from the
bottom, with wrap results reused by width when justified. This is secondary to
the output/update bottlenecks but matters with large side panes.

## Names and ownership

| Current | Recommendation | Reason |
|---|---|---|
| `outputController` | `transcript` | It owns scrollback, prompt, wrapping width, and viewport; it does not control input |
| `paneResource` | `pane` | “Resource” adds no useful distinction in this private interface |
| `paneRegistry` | Keep one small owner, optionally `paneStore` | Lazy creation and preserving output identity are real invariants; avoid scattering raw map access |
| `surface` in layout nodes | `widget` | It is already a `widget.Widget`; remove the extra vocabulary |
| Private `layoutNode` | `resolvedNode` | Distinguishes allocated/active state from declarative `ui.LayoutNode` |
| `layoutPlan` | Keep | One geometry result shared by interaction and painting is valuable |
| `frameGrid`, `frameEdges` | `borderGrid`, `borderEdges` | “Frame” also means a screen update; these describe borders |
| `frames.go` | `borders.go` | Own connectivity, junctions, and positioned border cells together |
| `compose`, `composeTick`, `stale` | `paint`, `paintTick`, `dirty` | Separates screen painting from the multiline input composer |
| `layout_size.go` | `layout_measure.go` | Its responsibility is measurement/allocation; retain this separation from placement |
| `util.VisibleLen` | Use `ansi.StringWidth` directly | The wrapper adds neither behavior nor clarity, and “length” obscures display columns |
| `ScrollbackBuffer` | `Scrollback` | The buffer suffix contributes little; keep its bounded ring and stable sequence numbers |

Do not rename everything before the performance changes. Use these names while
changing the corresponding ownership or code, keeping each review focused.

**Keep the real distinctions:** declarative layout versus resolved geometry;
buffer contents versus placement; output's append-time physical rows versus
side panes' resize-reflowed logical lines; scrollback versus its visible window.
Collapsing those behaviors into a universal pane would likely add flags and
branches. A shared scroll-position helper could be worthwhile, but preserving
the different line units is essential.

The `Widget` measurement/size/view contract and `inputController` also earn
their place: the controller centralizes submission and picker-callback
invariants, and its mode is derived rather than a second mutable state machine.
Search's viewport snapshot/restore ownership is useful. Do not merge picker
filtering and transcript search merely because they both display result rows.

**Collapse repeated mechanisms:** `surfaceFrames` discovers border edges by
asking a widget to regenerate full rules and labels, and `planFrames` asks
again. Compute the widget's geometry once and reuse it for measurement,
placement, and border drawing. Default separators can become border rules;
custom-character separators still need their drawing behavior. Removing the
`Separator` widget should follow unifying those paths, not discard that feature.

**Keep small, useful files:** `render.go`, border connectivity, layout placement,
and measurement have separate responsibilities. Combining their roughly 1,100
lines would not remove concepts. `widget/text.go` is a better merge candidate:
its single clipping helper can live with related ANSI utilities if that leaves
one clear owner. Tiny forwarding methods are lower priority than avoiding
per-message reconstruction of the entire scene.

**Skip unchanged work:** successful bar snapshots are pushed every 250 ms even
when unchanged, and every such message marks the whole screen stale. Compare
content before requesting paint; distinguish bar visibility changes from text
changes. Likewise, scrolled output can retain its cached visible rows when new
appends have not evicted that window. These are explicit invalidation rules,
not reasons to add a general-purpose dependency framework. The compositor's
on-demand tick does not make the whole application free of idle wakeups.

## Acceptance for the next changes

Use [the rendering harness](../render-performance.md) before and after each
step. The primary results are incoming-output latency and burst drain time;
the supporting results explain allocations and CPU. UI race tests and cell
snapshots must pass. A useful first milestone is that an unrelated draft's
length no longer determines the cost of an incoming output message. The next
is that ordinary output performs no geometry or border-grid reconstruction.
Measure scheduling and representation changes only after those wins are clear.
