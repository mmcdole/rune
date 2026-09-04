# Tree layout simplification

Status: implemented and verified.

## Scope

Keep Lua responsible for layout policy, Session for serialized Lua execution
and snapshot publication, and the TUI for measurement, allocation, interaction,
and painting. Simplify this branch before merging; do not retain aliases for
APIs introduced only during its development.

Bars retain terminal-width callbacks. No allocated-width feedback protocol.

## Implementation

- Normalize omitted sizes once: `1fr` except input/bar/separator heights in
  columns, which use `auto`. Explicit automatic widths remain unsupported.
- Allow empty and singleton containers without flattening their identity or
  constraints. Preserve strict fields, resource-name uniqueness, bounds, and
  the exactly-one-input invariant.
- Measure pane automatic height from content at the assigned inner width,
  bounded by terminal height and explicit maximum. Remove the ten-row default.
- Replace local `is_visible` queries with `is_hidden`. Ancestor visibility is
  not folded into local state. Regions containing input can be identified but
  cannot be hidden.
- Share copy-on-write visibility traversal for pane names and region IDs.
- Install the normal layout in Lua core; retain minimal Go recovery geometry.
- Give leaves one surface contract for minimum size, read-only measurement,
  size application, and rendering. Remove the pane adapter, leaf-kind enum,
  and duplicate resolved/placed node representations.
- Resolve and apply final geometry after each update. View paints that plan;
  it does not resize widgets or change navigation. Generate titles from live
  state rather than caching them in geometry.
- Protect input minima on both axes. Under pressure, drop gaps and relax other
  minima without overriding explicit maxima. Surfaces declare their minima;
  allocation does not prioritize a resource by name.
- Share nested frame boundaries independently of fractional sizing. Join pane
  frames, dividers, separators, and input rules through one frame grid. Clip
  titles before junctions.
- Consolidate documentation: guide examples, precise reference contracts, one
  migration section, and a short ownership explanation in architecture docs.

## Regressions and verification

The review reproduced three failures:

1. A fixed 100-column sidebar removed nested input at terminal width 100.
2. Search followed by multiline `SetInput` retained the old input height.
3. Fixed-size stacked panes produced a doubled seam beside a full-height pane.

The supplied screenshot suggests missing tees at the upper/lower band boundary
and at the central input's edges. Different split positions between bands are
valid. This interpretation is not a confirmed diagnosis of that user's build.

Use a synthetic fixture with this topology:

```text
column
├── row: map | chat | targets
├── separator
└── row: stats | column: output, input | group
```

Verify exact rectangles and junctions, constrained terminals, content-based
measurement, immutable normalization, local visibility, reload behavior, and
read-only measurement/painting. Run package tests, race checks, and the real
terminal fixture in an isolated tmux server without connecting to a MUD.

Keep transcript append-time wrapping, output batching, search anchors, pane
buffers, and bar lifetimes unchanged. Do not add a second effective-visibility
API, generic layout framework, or cache invalidation system.
