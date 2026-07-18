package tui

import (
	"strings"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func (m *Model) getLayout() ui.LayoutConfig {
	if len(m.luaLayout.Top) > 0 || len(m.luaLayout.Bottom) > 0 {
		return ui.LayoutConfig{
			Top:    m.luaLayout.Top,
			Bottom: m.luaLayout.Bottom,
		}
	}
	return ui.DefaultLayoutConfig()
}

// getWidget returns the Widget for a given name.
func (m *Model) getWidget(name string) widget.Widget {
	// Check widgets map (input, separator, bars)
	if w, ok := m.widgets[name]; ok {
		return w
	}

	// Panes (PaneManager returns *Pane which implements Widget)
	if m.panes.Exists(name) {
		return m.panes.Get(name)
	}

	return nil
}

// layoutDock sizes and renders one dock's widgets in a single pass,
// returning the joined view and the dock's total height. A widget with
// PreferredHeight 0 (hidden bar, collapsed pane) is skipped entirely.
func (m *Model) layoutDock(entries []ui.LayoutEntry) (string, int) {
	var parts []string
	totalHeight := 0

	for _, entry := range entries {
		w := m.getWidget(entry.Name)
		if w == nil {
			continue
		}

		// Width can affect intrinsic height (notably soft-wrapped composer
		// text), so make the current width available before asking for it.
		// Existing fixed-height widgets ignore the zero height.
		w.SetSize(m.width, 0)
		preferred := w.PreferredHeight()
		if preferred == 0 {
			continue
		}

		h := entry.Height
		if h == 0 {
			h = preferred
		}

		w.SetSize(m.width, h)
		parts = append(parts, w.View())
		totalHeight += h
	}

	return strings.Join(parts, "\n"), totalHeight
}

// View implements tea.Model.
// Layout is calculated here to ensure it's always fresh when rendering.
func (m *Model) View() string {
	if !m.initialized {
		return "Loading..."
	}

	// Calculate layout fresh each render - guarantees no stale dimensions
	cfg := m.getLayout()
	topView, topHeight := m.layoutDock(cfg.Top)
	bottomView, bottomHeight := m.layoutDock(cfg.Bottom)

	viewportHeight := m.height - topHeight - bottomHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	// The viewport spans the full terminal width; splitRows wraps
	// appended rows to the same m.width.
	m.viewport.SetSize(m.width, viewportHeight)

	var parts []string
	if topView != "" {
		parts = append(parts, topView)
	}
	parts = append(parts, m.viewport.View())
	if bottomView != "" {
		parts = append(parts, bottomView)
	}

	return strings.Join(parts, "\n")
}
