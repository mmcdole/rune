package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

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

// sizeDockEntry applies one layout entry and returns its widget and final
// height. Measurement and rendering share this path so intrinsic-height
// policy cannot drift between an Update-time reflow and the next View.
func (m *Model) sizeDockEntry(entry ui.LayoutEntry) (widget.Widget, int, bool) {
	w := m.getWidget(entry.Name)
	if w == nil {
		return nil, 0, false
	}

	// Options are per-entry but the widget instance is shared, so pass the
	// bag unconditionally: an entry without options must reset whatever a
	// previous entry configured in the same layout pass.
	if c, ok := w.(widget.Configurable); ok {
		c.SetOptions(entry.Opts)
	}

	// Width can affect intrinsic height (notably soft-wrapped composer text),
	// so make the current width available before asking for it. Existing
	// fixed-height widgets ignore the zero height.
	w.SetSize(m.width, 0)
	preferred := w.PreferredHeight()
	if preferred == 0 {
		return nil, 0, false
	}

	h := entry.Height
	if h == 0 {
		h = preferred
	}
	w.SetSize(m.width, h)
	return w, h, true
}

// layoutDock sizes and renders one dock's widgets, returning the joined
// view and total height. A zero-height widget is skipped entirely.
func (m *Model) layoutDock(entries []ui.LayoutEntry) (string, int) {
	var parts []string
	totalHeight := 0
	for _, entry := range entries {
		w, h, ok := m.sizeDockEntry(entry)
		if !ok {
			continue
		}
		parts = append(parts, w.View())
		totalHeight += h
	}
	return strings.Join(parts, "\n"), totalHeight
}

// dockHeight measures a dock without rendering it. Search uses this during
// Update so viewport positioning sees the same final geometry as View.
func (m *Model) dockHeight(entries []ui.LayoutEntry) int {
	totalHeight := 0
	for _, entry := range entries {
		_, h, ok := m.sizeDockEntry(entry)
		if ok {
			totalHeight += h
		}
	}
	return totalHeight
}

func (m *Model) setViewportSize(topHeight, bottomHeight int) {
	viewportHeight := m.height - topHeight - bottomHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.SetSize(m.width, viewportHeight)
}

// syncViewportSize makes layout geometry current outside View. Callers that
// anchor content inside the viewport can then position it against the same
// dimensions the next render will use.
func (m *Model) syncViewportSize() {
	if !m.initialized {
		return
	}
	cfg := m.getLayout()
	m.setViewportSize(m.dockHeight(cfg.Top), m.dockHeight(cfg.Bottom))
}

// View implements tea.Model.
// Layout is calculated here to ensure it's always fresh when rendering.
func (m *Model) View() tea.View {
	view := tea.View{
		AltScreen: true,
		MouseMode: tea.MouseModeCellMotion,
	}
	if !m.initialized {
		view.Content = "Loading..."
		return view
	}

	// Calculate layout fresh each render - guarantees no stale dimensions
	cfg := m.getLayout()
	topView, topHeight := m.layoutDock(cfg.Top)
	bottomView, bottomHeight := m.layoutDock(cfg.Bottom)

	// The viewport spans the full terminal width; splitRows wraps
	// appended rows to the same m.width.
	m.setViewportSize(topHeight, bottomHeight)

	var parts []string
	if topView != "" {
		parts = append(parts, topView)
	}
	parts = append(parts, m.viewport.View())
	if bottomView != "" {
		parts = append(parts, bottomView)
	}

	view.Content = strings.Join(parts, "\n")
	return view
}
