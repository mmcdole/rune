# Rendering Simplification PRD

Status: Section 3, the scoped Section 4 contraction, and the Section 5 frame lifecycle cleanup are approved and implemented. Other implementation proposals remain unapproved.

Review baseline: `simplify-rendering`, commit `17b7df0`, September 20, 2026.

## Purpose

Make Rune's rendering path easier to understand, cheaper to execute, and more consistently named without changing established editing, scrolling, wrapping, or terminal behavior.

This is one PRD for the complete proposal. Its numbered sections are independent discussion units, not a commitment to implement every idea. Each section distinguishes an observed problem from a proposed solution and identifies the behavior that must survive the change.

An abstraction is a removal candidate when it duplicates state, reconstructs information already available, exposes operations that consumers do not use, or forces conversions without providing a necessary behavior. A short interface or a separate file is not, by itself, evidence of a useless abstraction.

## Review process

1. Explain one numbered section in plain language, including the proposed change and its principal tradeoff.
2. Discuss that section until the user agrees with its scope and direction. Revise this document when the discussion changes the proposal.
3. Record the section's decision here before advancing. Sections may be approved, revised, deferred, or rejected independently.
4. Keep implementation separate from discussion. Do not start changing application code merely because this PRD exists. Follow the user's instruction about when to implement an agreed section.
5. When implementing, take one agreed section at a time, with the dependencies listed below. Validate its behavior and performance before adding another change.

The first approved implementation is Section 3: separate geometry from appearance. Discussion was reprioritized around readable, minimal layout/rendering code and Rune's usual workload of single-line commands and occasional 12–24-line pastes. The thousand-line draft measurements remain diagnostic evidence, not a reason to add caching. Section 1 is deferred; subsequent sections require their own discussion and agreement.

## Scope and evidence

The analysis covered the TUI model, rendering and layout code, widgets, relevant text utilities, existing tests and benchmarks, and installed Bubble Tea/Ultraviolet source. No Markdown documentation or session history was used for the review. Session implementation was outside the review scope; proposals affecting UI events must preserve the existing event contract unless separately examined and agreed.

Application source files were not changed during analysis. Two temporary Go overlays tested draft string caching and bypassing composition-canvas cell-change tracking.

### Current path

```text
UI message
  -> Model.Update / dispatch
  -> optional layout resolution and widget sizing
  -> search-position adjustment
  -> render scheduling
  -> widget ANSI strings
  -> Rune cell canvas, joined borders, and labels
  -> full-screen ANSI string
  -> Bubble Tea cell canvas
  -> terminal diff and encoding
```

The production path renders from `Update` when the throttle permits it. With a zero render interval, `View` performs rendering instead. Terminal flushing has its own clock inside Bubble Tea.

### Measurements from the review

These are approximate local measurements on an AMD Ryzen 7 4800U, not cross-machine guarantees. Benchmarks measure different units and should not be added together.

| Workload | Observed result | Interpretation |
| --- | --- | --- |
| Ordinary output update while throttled | Approximately 55 ns, zero allocations | Existing fixed-geometry append handling is already inexpensive |
| 270x66 layout resolution | Approximately 16–31 microseconds | Layout is much cheaper than a frame, but needless repeated layout creates allocation pressure |
| 270x66 Rune frame composition | Approximately 2.1 ms | The full composition pass is a material cost |
| 270x66 prompt-change terminal pipeline | Approximately 4.9 ms and 9 terminal bytes per operation | Internal screen processing dominates the tiny resulting terminal update |
| Left/Right pair in a 1,000-line draft, with rendering throttled | Approximately 1.15 ms and 254 KB allocated | Cursor movement performs substantial document-size-dependent work before rendering |
| Same cursor workload with temporary string caching | Approximately 0.103 ms and 90 KB allocated | A small change removed most of the observed CPU cost |
| Unchanged named pane, 66 rows | Approximately 6.4 microseconds and 68 allocations | Rebuilding its string has a cost, but it is much smaller than full-screen composition |

In the cursor profile, rune-to-string conversion accounted for approximately 84% of sampled CPU time. Draft value conversion and border-grid allocation accounted for most sampled allocation volume. Those percentages describe that workload, not all application activity.

The terminal pipeline benchmark includes conversion, diffing, and encoding to an in-memory writer. It excludes physical terminal display latency and the production scheduling clocks. The canvas tracking experiment did not show a convincing repeatable improvement.

Baseline validation passed: `go test ./ui/... ./text/...`. Existing TUI, utility, and widget tests also passed with the temporary draft-value cache. Passing these tests is evidence for the experiment, not a substitute for reviewing the final implementation.

## 1. Reuse draft text and make edit detection explicit

**Status:** Deferred after workload discussion. **Priority:** Low for the expected workload. **Evidence:** Measured stress-case bottleneck and successful isolated experiment; no draft cache is approved or implemented.

Decision context: preserve the simple rune buffer. Consider explicit edit-change reporting when it simplifies the controller; do not add string caches or revision machinery solely to optimize thousand-line drafts. The original cache proposal below is retained as an investigated alternative, not the current implementation direction.

### Problem

`draftEditor` stores editable text as `[]rune`, while `Value()` produces a new string every time. `inputController` reads the value before an operation and again afterward to determine whether text changed. Cursor-only movement therefore reconstructs an entire unchanged draft twice per key.

The editor already invalidates its shaped layout when text changes. String conversion currently ignores that distinction between text changes and cursor changes.

Relevant code: `ui/tui/widget/draft_editor.go` (`Value`, `invalidate`), `ui/tui/input_mode.go` (`handleDraftEditorKey`, `forwardToInput`, `reportInputUpdate`).

### Proposal

- Cache the string representation of the structured draft. Rebuild it only after a text mutation.
- Invalidate the string through the same centralized mutation path used for other text-derived state.
- Keep cursor movement, selection changes, and width changes from invalidating the string unless they also change text.
- Preserve the existing `Value()` API and existing UI events for the first implementation.
- As a separate choice within this section, consider having editing operations report whether text or cursor changed. That would let the controller avoid comparing whole values. Do not combine this larger contract change with the cache unless its benefit and compatibility are clear.

### Required behavior

- Every insertion, deletion, replacement, reset, paste, and editor transition returns the correct current text.
- Text-edit events remain distinct from cursor-movement events. Cursor-only navigation must not start emitting text-edit events.
- Normal single-line input behavior and submission interpretation remain unchanged.
- History recall, selection replacement, and external editor interactions retain their current semantics.

### Acceptance criteria

- Repeated reads after cursor-only movement reuse the existing string without document-sized allocation or rune re-encoding.
- The existing large-draft cursor benchmark shows a substantial repeatable reduction from the baseline; the temporary experiment suggests roughly an order-of-magnitude CPU improvement is feasible.
- Mutation and event tests cover stale-cache risks rather than merely checking that a cache field exists.
- Remaining allocations, including layout allocation, are reported separately rather than attributed to the cache fix.

### Tradeoff and boundary

The editor retains a string alongside its rune slice, increasing retained memory while reducing repeated transient allocation. The cache is private derived state, never a second editable source of truth. Keep invalidation centralized to prevent stale text.

## 2. Invalidate rendering and layout according to actual changes

**Status:** Proposed. **Priority:** High. **Dependency:** Coordinate with Sections 3 and 5.

### Problem

`dispatch` returns two booleans, but many unrelated messages fall through to `return true, true`. Keyboard input, mouse scrolling, cursor updates, pane replacement/clearing, bindings, and configuration can therefore request a full layout rebuild even when rectangles are unchanged.

Every applied layout creates a new plan and border grid and sets `needsClear`. In addition, `PaneWriteMsg` requests rendering even when its pane is unplaced. Bar content changes always request layout even if an already-visible one-row bar remains visible.

Relevant code: `ui/tui/model.go` (`dispatch`, `Update`, `syncBars`), `ui/tui/layout.go` (`applyLayout`, `collectAutoPanes`).

### Proposal

- Replace opaque boolean pairs with a small named result that expresses paint and geometry changes. Choose a plain struct or similarly small representation; do not introduce an event-processing framework.
- Have handlers describe the actual effect of their operation, including no-ops.
- Reuse layout for cursor movement, scrolling within existing dimensions, and text changes that cannot alter a track's size.
- Retain layout invalidation for terminal resizing, placement changes, visible widget appearance/disappearance, and changed content measurement in auto-sized placements or ancestors.
- Use visibility and geometry dependency information from the resolved plan where useful. Avoid creating multiple independently maintained registries of the same placement state.
- Treat reportable state changes separately from whether pixels changed; an invisible update may still have a required UI event.

### Required behavior

| Trigger | Expected work |
| --- | --- |
| Unchanged prompt or unchanged bars | No paint or layout work |
| Cursor movement in a stable input area | Update cursor/viewport state and paint; preserve outer layout |
| Ordinary output scroll | Paint changed output; preserve outer layout |
| Write to an unplaced pane with no visible effect | Update buffer; no screen work |
| Write to an auto-sized visible pane | Re-evaluate relevant geometry when measurement may change |
| Replace or clear a fixed-size pane | Paint without unrelated resizing, unless another effect changes geometry |
| Bar text changes while its visibility is unchanged | Paint; no layout solely because the text changed |
| Empty bar becomes visible, or visible bar becomes empty | Resolve layout |
| Input overlay opens/closes or measured height changes | Resolve layout and adjust navigation geometry before use |

### Acceptance criteria

- Existing tests for fixed versus auto-sized pane updates continue to pass.
- Meaningful probes demonstrate that cursor movement and output scrolling do not resize unrelated widgets.
- Hidden-pane writes and harmless no-ops do not schedule unnecessary frames.
- Shrinking content, removing placements, and changing borders erase old pixels correctly.
- Search previews and restored positions use final geometry.
- Ordinary throttled output updates retain their existing low overhead.

### Tradeoff and boundary

More precise invalidation can produce stale content if dependencies are missed. Keep the rules explicit and conservative where measurement is uncertain. Do not defer every layout operation until a frame: editing and append-time wrapping sometimes need current dimensions immediately.

`Input.SetSize` currently also adjusts the draft's visible top row. Before skipping repeated sizing, make cursor visibility an explicit editing/viewport responsibility so navigation continues to scroll the draft correctly.

## 3. Separate changing labels from stable geometry

**Status:** Approved and implemented. **Priority:** First agreed cleanup. **Dependency:** Enables safe refinements in Section 2; does not itself change dispatch invalidation.

### Implemented contract

- `Widget` retains its existing sizing and content methods.
- `Input.Rules(width, height)` returns geometry for proposed dimensions.
- `Input.Labels()` returns current styled text and local positions at the applied dimensions.
- `Rule` contains only line geometry; `RuleLabel`, `Rule.Labels`, and `Input.LabeledRules` are removed.
- `Rule.Translate` translates only coordinates; label-slice copying and nested label translation are removed.
- Layout marks dividers and leaf rules directly into the border grid. The intermediate `layoutPlan.rules` list is removed.
- Rendering draws content, restores borders, and obtains fresh pane titles and input labels. The renderer translates and clips labels.
- `input_labels.go` owns label data, styling, and hint formatting. Those responsibilities were removed from `input_layout.go`, `draft_editor_layout.go`, and `rule.go`. Label tests now live in `input_labels_test.go`.
- No new widget interface, label manager, or cache was introduced.

Validation: existing render snapshots pass unchanged. New integration cases compare painting with a reused layout against a fresh layout after mode, discard-warning, line-count, binding-hint, and label-removal changes. They cover shared borders, nonzero input origins, and narrow terminals. `go test -race -shuffle=on ./ui/... ./text/...`, `go vet ./ui/... ./text/...`, and diff whitespace checks passed.

### Problem

`planBorders` calls `LabeledRules`, formats draft labels, translates them, and stores them in the layout plan. Pane titles are instead obtained during drawing. These are two different lifecycles for equivalent decorative text.

A binding hint, submission mode, or discard warning can change without moving a border. Baking labels into the plan ties those changes to layout rebuilding. Simply making invalidation more selective could leave stale labels behind.

Relevant code: `ui/tui/borders.go` (`planBorders`), `ui/tui/render.go` (`drawLabels`), `ui/tui/widget/input_layout.go` (`LabeledRules`), `ui/tui/widget/draft_editor_layout.go` (`draftLabels`).

### Proposal

- Store rectangles, rule positions, and shared-border connectivity in the layout plan.
- Obtain current labels during painting, using the final allocated geometry.
- Apply this lifecycle consistently to pane titles and input labels.
- Compute label placement from existing geometry without invoking global layout resolution.
- Prefer straightforward label production over another cache until profiling establishes a need.

### Required behavior

- Mode labels, line counts, binding hints, and discard warnings update when their state changes.
- An explicit pane title continues to override its generated title.
- Labels remain clipped to available horizontal space and do not overwrite junctions.
- Replacing a long label with a short label, or removing one, restores the underlying border.
- Tiny layouts preserve existing label omission and input reachability behavior.

### Acceptance criteria

- A label-only change becomes visible without rebuilding global geometry.
- Snapshot coverage includes changing and removing labels at unchanged dimensions.
- Measurement performs no label formatting or style rendering.
- There is one clear owner for final label painting.

### Tradeoff and boundary

If unchanged leaves are later skipped during painting, restoring the old label footprint becomes explicit work. The current full border repaint handles this implicitly. Preserve that behavior until a selective painting design replaces it deliberately.

## 4. Simplify decoration, measurement, and placement contracts

**Status:** Scoped contraction approved and implemented. Remaining proposals in this section are unapproved. **Priority:** Medium. **Dependency:** Builds on Section 3.

### Agreed scope and implemented result

The goal is fewer layers and less duplicated work while preserving the existing layout behavior. The user approved contracting the implementation, including reorganizing files when that makes the flow easier to follow.

- Removed `resolveWidget`, `resolveBar`, and `resolvePane`. `resolveNode` directly handles each supported leaf type and returns a node or `nil`; the redundant success boolean is gone.
- Combined child resolution and input-descendant tracking into one loop.
- Input resolution measures its preferred height at most once, and skips that measurement when a vertical track already supplies a fixed height. It calculates the initial border edges once. Placement still checks borders at the actual allocated size, because constrained inputs can drop decorations.
- Bars use their existing one-row measurement to collapse absent or empty content, including style-only content. Separators no longer pass through generic visibility and border measurement.
- Measurement and placement now call the same `contentInsets` formula with explicit node type, own edges, and shared edges. Their different provisional and final dimensions remain explicit.
- Moved border edge definitions, border discovery, and inset calculation into `borders.go`. `layout.go` reads as resolution followed by placement; `layout_measure.go` handles sizing.
- Kept the existing small `Widget` contract. No new visibility interface, cache, factory, or intermediate representation was introduced.

The resulting contract is: measurement answers sizing questions without applying geometry; layout selects active nodes and assigns rectangles; widgets accept their final dimensions; rendering draws content, joined borders, and current labels. Initial border measurement and final border checks serve different constraints and are deliberately retained.

Validation: `go test -race -shuffle=on ./ui/... ./text/...`, `go vet ./ui/... ./text/...`, and `git diff --check` pass. Added a regression that toggles a fixed-height bar subtree through visible, empty, style-only, and removed states, verifying reclaimed space and rendered content. Existing layout and rendering tests cover shared borders, nested allocation, hard caps, and small terminals. This is a structural cleanup; no quantitative speedup is claimed.

The broader proposals below remain discussion material. This implementation does not authorize another interface, caching, or a general layout redesign.

### Problem

The renderer discovers `Rules` and `LabeledRules` through separate anonymous interfaces. Only `Input` currently implements these decoration queries. It repeatedly creates an `inputLayout` for measurement, border discovery, final sizing, labels, and content.

`resolveWidget` also uses `MeasureHeight` to decide whether a widget exists in the layout, although empty-bar collapse is the immediate reason for that check. Requested borders, anticipated shared borders, and final placement insets are related but are computed in several places.

Relevant code: `ui/tui/widget/widget.go`, `ui/tui/widget/input_layout.go`, `ui/tui/layout.go` (`resolveWidget`, `widgetBorders`, `placeNode`), `ui/tui/layout_measure.go` (`contentInsets`, `assignSharedEdges`, allocation helpers).

### Proposal

- Make the decoration contract explicit in one place, with a clear distinction between geometry and labels.
- Compare a small named optional interface with direct handling of the sole decorated widget. Select the option that reduces actual branching and duplication; do not add an interface merely to rename the anonymous ones.
- Reuse the final input layout for the allocated dimensions across sizing and rendering when its structural inputs are unchanged.
- Keep provisional measurement independent from the final editing-width cache. A measurement at another width must not evict useful final layout state.
- Consider making empty-bar visibility explicit rather than relying on a generic measurement call to discover whether every widget exists.
- Consolidate repeated inset rules where possible, while retaining the distinction between proposed edges and edges that survive constrained allocation.

### Acceptance criteria

- A reader can identify the source of each final widget rectangle and border without reconstructing several overlapping contracts.
- Measurement does not alter navigation-visible dimensions or cursor position.
- Existing tests for nested layouts, shared edges, dividers, hard caps, and tiny-terminal fallback continue to pass.
- Reuse avoids demonstrable repeated work and has a complete, understandable invalidation rule.
- No new general-purpose layout engine or widget hierarchy is introduced.

### Tradeoff and boundary

Some multiple-pass work is necessary: wrapped height depends on allocated width, and constrained layouts may drop borders. Do not collapse provisional and final geometry into one value or assume that every repeated query is redundant.

## 5. Unify rendering lifecycle and separate reporting retries

**Status:** Proposed. **Priority:** Medium. **Dependency:** Coordinate with Section 2; preserve event timing.

### Problem

Production rendering runs in `Update` through `renderThrottled`, while zero-interval operation renders on every `View`. Tests and benchmarks consequently exercise a different rendering lifecycle.

`renderThrottled` also sets pixel dirtiness when scroll-state reporting fails. A full event queue can therefore cause another full render even if only reporting needs to be retried.

Relevant code: `ui/tui/model.go` (`renderThrottled`, `reportScrollState`), `ui/tui/render.go` (`View`, `render`), rendering and output benchmarks.

### Proposal

- Use one render/flush operation for all modes. Change when it is scheduled, not which lifecycle method owns rendering.
- Make `View` return the prepared view consistently.
- Support immediate deterministic flushing for tests without making every view read a repaint request.
- Track pending scroll-state delivery independently from dirty pixels. Reuse the existing comparison against the last accepted state where sufficient, rather than introducing redundant stored state.
- Schedule another wakeup while required render or reporting work remains, and stop scheduling once both are settled.

### Required behavior

- The first visible change after idle remains responsive.
- Bursts remain coalesced and at most one Rune render tick is outstanding.
- A reporting retry does not repaint an unchanged screen.
- Scroll-state reports retain their relationship to the prepared screen; the refactor must not report an intermediate unrendered state by accident.
- Resize, initialization, zero-sized terminals, and shutdown retain correct behavior.

### Acceptance criteria

- Tests can exercise the same ordering as production with deterministic scheduling.
- Reading `View` repeatedly does not mutate widget state or increase the render count.
- A full UI-event queue produces bounded retries and stops generating work after delivery succeeds.
- Existing terminal-output probes continue to acknowledge actual rendered output correctly.
- Benchmarks explicitly state whether one operation includes update, composition, terminal encoding, or scheduling latency.

### Tradeoff and boundary

Some tests modify widget state directly and then call `View`. They will need to invoke the explicit render path. Adapt tests to the production contract rather than retaining a second rendering architecture solely for test convenience.

Do not remove Rune's throttle merely because Bubble Tea has a terminal flush clock: that clock does not by itself eliminate Rune's composition cost.

## 6. Reduce the full-screen string and cell conversion chain

**Status:** Proposed investigation. **Priority:** Largest architectural opportunity; implement only after comparing prototypes.

### Problem

Widgets create styled strings. Rune parses those strings into cells, adds borders and labels, and serializes the entire canvas to an ANSI string. Bubble Tea parses that result into another cell buffer before diffing and encoding terminal output.

This repeats screen traversal, ANSI parsing/encoding, width handling, and cell processing. A tiny prompt change can still require processing most of the screen internally.

Relevant code: `ui/tui/render.go`, widget `View` methods, `ui/tui/render_bench_test.go` (`BenchmarkRenderPipeline`), installed Bubble Tea `cursed_renderer.go` and Ultraviolet buffer/styled-string code.

### Proposal: compare two concrete directions

| Direction | What it removes | Principal cost or risk |
| --- | --- | --- |
| Compose styled rows for Bubble Tea | Rune's full-screen ANSI-to-cells-to-ANSI conversion | Rune must preserve clipping, gaps, wide graphemes, border joins, and style boundaries during row composition |
| Keep cells through terminal rendering | Full-screen serialization and reparsing between Rune and the terminal renderer | The installed Bubble Tea view path consumes strings; integration changes must preserve terminal and input behavior |

- Prototype the narrowest useful path for each viable direction using identical fixtures.
- Choose one dominant representation based on measured benefit and total implementation complexity.
- Prefer eliminating a conversion boundary over building a framework that supports both representations indefinitely.
- Consider a hybrid only if it has a concrete, measured advantage that outweighs its additional ownership and invalidation rules.

### Required behavior

- Preserve output wrapping fixed at append time and named-pane reflow on resize.
- Preserve styled text, supported links, grapheme widths, clipping, tabs, control visualization, and cell cleanup.
- Preserve joined borders, separators, titles, labels, gaps, and constrained layouts.
- Preserve alternate-screen behavior, mouse and keyboard handling, resize behavior, and terminal shutdown cleanup if the integration boundary changes.

### Decision and acceptance criteria

- Compare composition time, complete pipeline time, allocations, terminal bytes, and the relevant latency benchmarks.
- Cover prompt-only changes, output scrolling/floods, cursor-only changes, nested panes, large drafts, and resizing.
- Run comparative benchmarks serially, with repeated samples and identical fixtures. Avoid claiming gains from concurrent benchmark runs or single noisy samples.
- Require a material, repeatable improvement in relevant workloads and a clear reduction in conversion/ownership complexity before selecting a replacement.
- Compare rendered cells or terminal-visible results where ANSI byte sequences legitimately change.
- Document any remaining conversion boundaries and the behavior they provide.

### Tradeoff and boundary

This is a design investigation, not evidence that either replacement is already simpler or faster. The existing canvas supplies valuable correctness. A fast prototype that loses clipping, Unicode, or border behavior does not meet the requirement.

## 7. Remove redundant accessors, wrappers, and separator lifecycle work

**Status:** Proposed. **Priority:** Low-risk cleanup for accessors; conditional for separators.

### Problem and candidates

| Candidate | Current redundancy | Proposed treatment |
| --- | --- | --- |
| `pane.Name()` | The internal interface requires a method that production consumers do not call; lookup already uses the registry key | Remove the requirement; evaluate concrete exported methods separately before removing them |
| `resolveBar` | Adds a lookup-and-forward layer without a separate policy | Inline into node resolution if the resulting switch is clearer |
| Default `Separator` widget | Constructed and measured as a widget, then converted to a joined rule and stripped of its content rectangle | Resolve eligible default separators directly into rule geometry |
| `style.RenderBorder` helper | A generic-sounding styling helper has only the separator as a caller | Move or rename it with the separator styling cleanup in Section 9 |
| Exported `Pane.ContentRows` | Its production use is internal to `Pane.View` | Consider reducing visibility after checking external/API expectations; do not remove it solely because repository consumers are limited |

Relevant code: `ui/tui/panes.go`, `ui/tui/layout.go`, `ui/tui/borders.go` (`joinableSeparator`), `ui/tui/widget/separator.go`, `ui/tui/widget/pane.go`, `ui/tui/style/styles.go`.

### Required behavior

- Named buffers continue to be created and found consistently, including the distinguished output pane.
- Default separators retain border joining when placed in a column.
- Custom separators and separators placed in rows retain their existing rendering semantics.
- Hiding, sizing, and constraining a separator continue to obey layout rules.
- Cleanup does not force custom separator characters into the box-junction algorithm.

### Acceptance criteria

- Each removed layer eliminates a concrete indirection or lifecycle step.
- No larger replacement abstraction is needed merely to preserve the deleted helper's shape.
- Separator changes preserve existing layout and rendering tests, including row/column placement and custom characters.
- Exported-method changes receive a compatibility check before deletion.

### Tradeoff and boundary

The separator rewrite is optional. If direct rule resolution creates more special cases in measurement and placement than the widget currently requires, retain the widget and clean up its naming and styling instead.

## 8. Standardize internal names and document coordinate units

**Status:** Proposed. **Priority:** Cleanup alongside the relevant behavior changes.

### Problem

Most names are coherent, but some equivalent concepts have different names, and some names hide important coordinate units or lifecycle distinctions. These inconsistencies increase the effort needed to follow layout and scrolling code.

### Proposed vocabulary

| Current name or inconsistency | Proposed direction | Reason |
| --- | --- | --- |
| `Search.resultLines` versus `Picker.resultRows` | Use `resultRows` | Both produce physical display rows |
| `inputLayout.pickerHeight` | `resultsHeight` | The area also hosts search results |
| `layoutPlan.output` | `outputRect` | The value is a rectangle, not the output widget |
| `resolvedNode.edges` | `requestedBorders` or another explicit equivalent | These are requested boundaries used before final allocation |
| `resolvedNode.shared` | `anticipatedSharedBorders` or another explicit equivalent | The field describes measurement assumptions, not necessarily final insets |
| `style.RenderBorder` | `RenderSeparator` if retained | It renders the separator's horizontal line, not arbitrary pane borders |
| `ScrollPos` | Consider `ScrollSnapshot` | It includes mode and append counters as well as position |
| `newLines` and scrolling parameters | Explicit unit comments; rename internal output counts to rows where appropriate | Output stores physical rows while named panes scroll logical lines |

### Requirements

- Use **line** for a logical text line and **row** for a physical terminal row where that distinction matters.
- Use **cell/column** for display width and **rune offset** for the draft cursor. Do not silently interchange them.
- Use **rule** for a drawable line segment, **border** for a boundary edge or joined boundary, and **label** for text drawn over a rule.
- Keep provisional measurement names distinct from applied geometry names.
- Update comments and test names when changing terminology; avoid cosmetic renames unrelated to an agreed section.
- Preserve public event and Lua-facing names unless a separate compatibility change is explicitly approved.

### Acceptance criteria

- A targeted search shows consistent names for the agreed concepts.
- Scrolling APIs state whether their counts refer to logical lines or physical rows.
- Renames do not alter behavior, public schemas, or user configuration.

### Tradeoff and boundary

Prefer precise names over uniformly long names. The suggested spellings are proposals to discuss, not a requirement to rename every short field. Keep internal cleanup separate from external API churn.

## 9. Unify separator styling and display-width policy

**Status:** Proposed. **Priority:** Medium for consistency; verify behavior before changing validation.

### Problem

The custom/separately-rendered separator path uses hardcoded ANSI bright black through `style.RenderBorder`. Joined borders use `Styles.PaneBorder`, whose default is a different color specification. Related decoration therefore has different style ownership.

Separator validation uses `go-runewidth.StringWidth` in both `ui/layout.go` and `widget.Separator.SetChar`. Canvas composition uses ANSI grapheme-width handling. Different width policies can disagree for some Unicode sequences; the review established inconsistent policies, not a reproduced failure for every character category.

### Proposal

- Use the same configured border style for visually equivalent separator and border decorations.
- Remove the hardcoded separator color path or make its intentional distinction an explicit product choice.
- Select one display-width policy for separator acceptance and rendering.
- Preserve validation of the allowed separator character shape and controls separately from calculating visible width. An ANSI-aware width result alone must not accidentally broaden what input is accepted.
- Keep normalization-time validation and defensive Go-call validation aligned, ideally through a small shared predicate at an appropriate dependency boundary.

### Required behavior

- Joined and separately drawn default separators use the intended common style.
- Valid custom single-cell separators repeat without gaps or width drift.
- Empty, zero-width, multi-cell, combining, and multi-codepoint inputs have explicit, consistent acceptance or fallback behavior.
- Invalid direct Go calls remain bounded and cannot break layout width assumptions.
- Supported terminal controls and literal-control visualization retain their existing policies elsewhere.

### Acceptance criteria

- Compare style output for joined borders, default separators, and custom separators.
- Add focused width-policy cases only for meaningful acceptance and rendering boundaries.
- Validation and rendering agree on every newly covered accepted input.
- Any changed acceptance behavior is documented and reviewed as a behavior change rather than hidden inside a naming cleanup.

### Tradeoff and boundary

Unifying width policy may change which unusual strings are accepted. Decide that deliberately. A rendered-width check and a valid-separator check are related but not interchangeable.

## 10. Bound caching work and preserve abstractions that earn their place

**Status:** Proposed guardrails and deferred opportunities. **Priority:** Apply throughout; optimize only with evidence.

### Potential additional work

- `Pane.View` wraps visible logical lines and joins the rows on every call. A small cache keyed by buffer changes, size, and scroll position could avoid this work.
- `Bar.View` repeats width calculations and string assembly. Cache only if measured costs justify the additional state.
- `Output.View` caches its string, but Rune still parses that string into cells each rendered frame. Another widget-string cache will not eliminate that downstream cost.
- A retained canvas could skip painting unchanged leaves, but it must also handle shared borders, old labels, uncovered areas, and the full-screen serialization step.
- Rune's `ScreenBuffer` tracks changed cells, although Rune serializes the whole buffer rather than consuming its touched-line state. This is a candidate for structural cleanup, but bypassing that tracking did not produce a convincing repeatable gain in the review experiment.

### Proposal

- Prioritize Sections 1–5 before introducing broad widget caching or dirty-region machinery.
- Reassess caching after Section 6 chooses the representation, because a different composition boundary changes which caches are useful.
- Require each cache to have a clear owner, bounded memory, and a complete invalidation rule.
- Require selective painting to demonstrate pipeline gains, not merely fewer calls to widget `View`.
- Do not claim a performance benefit for removing unused canvas tracking without new evidence.

### Boundaries to preserve

| Existing boundary | Behavior it provides |
| --- | --- |
| `Output` versus named `Pane` | Output wraps at append time and keeps that historical shape; named panes retain logical lines and reflow on resize |
| `Scrollback` storage versus output viewport | Bounded storage and monotonic sequence anchors support stable search and scrolling across appends and eviction |
| Input controller versus input widget | Event/submission and picker callback policy are distinct from text editing, measurement, and drawing |
| Central shared-border resolution | Joins, overlaps, dividers, and tiny-terminal degradation need a consistent owner |
| Measurement versus applied geometry | Wrapped content may need provisional sizing without changing the geometry used by interaction |
| Rune composition scheduling versus terminal flushing | They limit different work; deleting one clock requires proof that its responsibility is preserved |

### Acceptance criteria

- No generic pane base class, renderer backend hierarchy, global cache manager, or generalized change-tracking framework is added solely to make types look uniform.
- Any retained cache survives append, clear, replacement, scrolling, resize, mode changes, and content shrink correctly where applicable.
- Performance claims identify whether they improve an isolated widget, composition, or the complete pipeline.
- Preserved abstractions may still be simplified internally, but their behavioral responsibilities remain explicit.

### Tradeoff and boundary

Skipping work often requires remembering more state. The goal is less total complexity and cost, not the maximum number of caches or the smallest number of types.

## Delivery and validation

### Discussion order

Section 3 was selected and implemented first after the workload and cleanup discussion, followed by the scoped Section 4 contraction and the Section 5 frame lifecycle cleanup recorded below. Section 1 is deferred. Review the remaining proposals one at a time; agreement on one section does not approve the others. Keep this document as the single source for decisions and revisions.

### Suggested implementation order after agreement

Section 5 implementation record: frame execution and scroll reporting are approved and implemented after an independent simplification review. Previously production rendered from `Update`, while zero-interval mode rendered on every `View`. Rejected scroll-state reports also set `dirty`, forcing redraws solely to retry delivery. Both sources of duplication are removed.

Implemented contract: `Update` applies state, geometry, and search positioning, then invokes `renderIfDue`; `View` only returns the prepared screen and terminal options. The helper returns while throttled, draws pending changes, attempts scroll reporting, and starts another interval only after drawing or rejected delivery. It owns clearing `dirty`; `render` draws. Report-only retries use the existing timer and last accepted scroll state without redrawing. Successful retries stop immediately when there are no new pixels. Zero interval renders pending changes during `Update`, schedules no timers, and retries failed reports on the next `Update`. No scheduler object, new interface, or extra pending field was added.

Code changes: renamed and modified the existing `renderThrottled` helper as `renderIfDue`, grouping it and the existing tick definition with drawing in `render.go`. `Update` invokes it; `View` no longer renders; `render` no longer clears `dirty`. `reportScrollState` remains the single delivery helper alongside event delivery. Fixtures that directly apply geometry now render explicitly, and the layout/frame benchmark still measures both operations. Message-driven tests continue through `Update`.

Validation: `go test -race -shuffle=on ./ui/... ./text/...`, `go vet ./ui/... ./text/...`, and `git diff --check` pass. Tests cover immediate first frames, burst batching, idle timer shutdown, repeated rejected reports with no extra redraws, successful retry shutdown, new changes waiting behind the throttle before reporting, side-effect-free `View` calls, and immediate zero-interval rendering with retries on subsequent updates. More precise per-message layout invalidation remains a separate discussion.

1. Completed: Section 3 and its rule/label contract cleanup, the scoped Section 4 implementation contraction, and the Section 5 frame lifecycle cleanup.
2. Discuss any further Section 4 proposals individually before implementing them.
3. Discuss Section 2's more precise invalidation using the clarified ownership; Section 5's execution and reporting lifecycle is complete.
4. Sections 7–9: take any subsequently agreed cleanup items individually, close to their owning code.
5. Section 6: investigate representation changes only if realistic workloads justify the complexity.
6. Sections 1 and 10: revisit deferred optimizations only with evidence from the expected workload.

Dependencies may change after discussion. Avoid packaging all sections into one large implementation change.

### Validation approach

- Run existing tests appropriate to the affected behavior, using `go test ./ui/... ./text/...` as the established broad baseline when rendering or text behavior changes.
- Use targeted behavior tests for cache invalidation, geometry changes, stale cells, event ordering, and Unicode boundaries. Do not write tests that merely mirror a new helper or field.
- Preserve render snapshots for ordinary input, draft editing, search, pickers, and scrolling; compare cells when serialization changes intentionally.
- Reuse existing render, cursor, auto-layout, resize, pipeline, and output-latency benchmarks as appropriate. Include keyboard, hidden-pane, and reporting-retry workloads when existing benchmarks do not exercise the changed path.
- Keep setup outside timing, describe the work represented by one operation, run competing variants serially, and use repeated samples.
- After each implementation, report changed behavior, measured improvement, validation results, and remaining limitations before advancing.

### Completion criteria

The work is complete when every numbered section has an explicit decision, every section selected for implementation meets its agreed acceptance criteria, and remaining deferred ideas are recorded with a reason. Completion does not require implementing speculative changes that fail to improve the code or measured workloads.
