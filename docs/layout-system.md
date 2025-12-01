# Layout System Design (v2 - Simplified)

This document captures the simplified design for Rune's UI layout system.

## Overview

The layout system allows Lua scripts to define custom UI layouts with:
- **Top dock**: Optional bars and panes (title, stats, etc.)
- **Main viewport**: Center scrollback area
- **Bottom dock**: Optional bars, panes, and built-in components
- **Overlays**: Inline pickers (slash, history, alias) that push content

**Explicitly deferred**: Side panes (left/right docks). These add significant complexity for a niche use case. Can be added in a future version.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   Top Dock                      │  ← Bars/panes
│              (title, stats, etc.)               │
├─────────────────────────────────────────────────┤
│                                                 │
│               Main Viewport                     │
│               (scrollback)                      │
│                                                 │
├─────────────────────────────────────────────────┤
│              Bottom Dock                        │  ← Bars/panes/built-ins
│         (combat log, input, status)             │
└─────────────────────────────────────────────────┘
```

## Core Types

### LayoutConfig

Declares which components go in each dock:

```go
type LayoutConfig struct {
    Top    []string // Components for top dock (rendered above viewport)
    Bottom []string // Components for bottom dock (rendered below viewport)
}
```

Both docks can contain any mix of bars, panes, and built-in components.

### Built-in Components

- `"input"`: The input line with overlays and separators (always includes top/bottom separator lines, plus overlay when active)
- `"status"`: The status bar (1 line)
- `"separator"`: A horizontal line (1 line)
- `"infobar"`: Lua-controlled info line, only shown if set (0-1 lines)

### BarDef

Defines a single-line bar (can be in top or bottom dock):

```go
type BarDef struct {
    Name   string
    Border Border  // BorderNone, BorderTop, BorderBottom, BorderBoth
    Render func(state ClientState, width int) BarContent
}

type BarContent struct {
    Left   string  // Left-aligned text
    Center string  // Center-aligned text
    Right  string  // Right-aligned text
}
```

### PaneDef

Defines a multi-line pane (can be in top or bottom dock):

```go
type PaneDef struct {
    Name       string
    Height     int    // Fixed height in lines
    Visible    bool   // Can be toggled
    BufferSize int    // Max lines retained
    Border     Border // Decorative borders
    Title      bool   // Show name as header
}
```

### Border

```go
type Border string

const (
    BorderNone   Border = ""
    BorderTop    Border = "top"
    BorderBottom Border = "bottom"
    BorderBoth   Border = "both"
)
```

## Height Calculation

Heights are calculated dynamically from the layout configuration:

```
totalHeight = terminalHeight
topDockHeight = sum of component heights in Top
bottomDockHeight = sum of component heights in Bottom

viewportHeight = totalHeight - topDockHeight - bottomDockHeight
```

### Component Height

Each component calculates its own height:

- **input**: 3 lines (separator + input + separator) plus overlay height when active
- **status**: 1 line
- **separator**: 1 line
- **infobar**: 1 line when set, 0 when empty
- **bars**: 1 line plus borders (0-2 lines)
- **panes**: configured height plus title (0-1) plus borders (0-2), or 0 if not visible

## Rendering Pipeline

Simple top-to-bottom rendering:

```go
func (m Model) View() string {
    // Recalculate viewport height (overlay state may have changed)
    topHeight := m.dockHeight(layout.Top)
    bottomHeight := m.dockHeight(layout.Bottom)
    viewportHeight := m.height - topHeight - bottomHeight
    m.viewport.SetDimensions(m.width, viewportHeight)

    var parts []string

    // 1. Top dock components
    for _, name := range layout.Top {
        parts = append(parts, renderComponent(name))
    }

    // 2. Main viewport (scrollback)
    parts = append(parts, m.viewport.View())

    // 3. Bottom dock components (including input with overlay)
    for _, name := range layout.Bottom {
        parts = append(parts, renderComponent(name))
    }

    return strings.Join(parts, "\n")
}
```

## Overlays

Overlays (slash picker, history search, alias picker) render **inline** as part of the input component. They push content up rather than floating on top.

When overlay is active, the input component renders as:
```
[overlay picker]
────────────────  (separator)
> input line
────────────────  (separator)
```

When no overlay is active:
```
────────────────  (separator)
> input line
────────────────  (separator)
```

The viewport height adjusts automatically because the input component's height includes the overlay.

## LayoutProvider Interface

Minimal interface for UI to get layout info:

```go
type LayoutProvider interface {
    // Layout returns current layout config
    Layout() LayoutConfig

    // Bar returns bar definition, nil if not found
    Bar(name string) *BarDef

    // Pane returns pane definition, nil if not found
    Pane(name string) *PaneDef

    // PaneLines returns current buffer for a pane
    PaneLines(name string) []string

    // State returns client state for bar rendering
    State() ClientState
}
```

## Example Layouts

### Default (separator + input + status)

```go
LayoutConfig{
    Bottom: []string{"input", "status"},
}
```

### With Title Bar

```go
LayoutConfig{
    Top:    []string{"title"},
    Bottom: []string{"input", "status"},
}
```

### With Stats and Combat Log

```go
LayoutConfig{
    Top:    []string{"title", "stats"},
    Bottom: []string{"combat", "input", "status"},
}
```

## Key Design Decisions

1. **No side panes** - Deferred to future version. Simplifies everything.

2. **Inline overlays** - Push content rather than float. More predictable for terminal UI.

3. **Explicit layout** - All components (including built-ins) are declared in the layout config.

4. **Dynamic height calculation** - All heights computed from layout, no hardcoded values.

5. **Simple top-to-bottom render** - No horizontal composition needed.

6. **Input includes separators** - The input component always has separator lines above and below for visual clarity.

## Future: Side Panes

When we add side panes later:
- Use `lipgloss.JoinHorizontal()` for left | center | right composition
- Side panes will need fixed widths
- Viewport width becomes: `termWidth - leftWidth - rightWidth`
- Keep the simple height model - side panes just share middle section height
