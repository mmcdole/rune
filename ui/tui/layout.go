package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/util"
	"github.com/mmcdole/rune/ui/tui/widget"
)

const (
	defaultPaneContentHeight = 10
	paneChromeHeight         = 2 // titled header + closing rule
)

// sizedDockEntry is the walker's private representation of one rendered
// layout entry. Exactly one of widget or pane is set.
type sizedDockEntry struct {
	widget widget.Widget
	pane   *widget.Pane
	height int
}

// dockPart is the compositor-facing form of a sized entry. The walker owns
// all pane-vs-widget dispatch; composition only deals with rendered blocks.
type dockPart struct {
	view                   string
	closing                string
	sharedWithPreviousPane bool
}

func (m *Model) getLayout() ui.LayoutConfig {
	if len(m.luaLayout.Top) > 0 || len(m.luaLayout.Bottom) > 0 {
		return ui.LayoutConfig{
			Top:    m.luaLayout.Top,
			Bottom: m.luaLayout.Bottom,
		}
	}
	return ui.DefaultLayoutConfig()
}

// sizeDockEntry resolves and sizes one layout entry. Named widgets take
// precedence over panes, matching the existing collision policy.
func (m *Model) sizeDockEntry(entry ui.LayoutEntry) (sizedDockEntry, bool) {
	if w, ok := m.widgets[entry.Name]; ok {
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
			return sizedDockEntry{}, false
		}

		h := entry.Height
		if h == 0 {
			h = preferred
		}
		w.SetSize(m.width, h)
		return sizedDockEntry{widget: w, height: h}, true
	}

	pane, ok := m.panes.Lookup(entry.Name)
	if !ok || !pane.Visible {
		return sizedDockEntry{}, false
	}
	height := entry.Height
	if height == 0 {
		height = defaultPaneContentHeight + paneChromeHeight
	}
	if height < paneChromeHeight {
		height = paneChromeHeight
	}
	return sizedDockEntry{pane: pane, height: height}, true
}

// walkDock is the single dispatch and geometry path for dock entries. When
// visit is non-nil it snapshots each entry's rendering before a repeated,
// shared widget can be resized by the next entry.
func (m *Model) walkDock(entries []ui.LayoutEntry, visit func(dockPart)) int {
	totalHeight := 0
	previousWasPane := false
	for _, entry := range entries {
		sized, ok := m.sizeDockEntry(entry)
		if !ok {
			continue
		}

		isPane := sized.pane != nil
		shared := previousWasPane && isPane
		if shared {
			totalHeight--
		}
		totalHeight += sized.height

		if visit != nil {
			part := dockPart{sharedWithPreviousPane: shared}
			if isPane {
				part.view = m.renderPaneBody(sized.pane, sized.height)
				part.closing = m.renderPaneClosing()
			} else {
				part.view = sized.widget.View()
			}
			visit(part)
		}
		previousWasPane = isPane
	}
	return totalHeight
}

func (m *Model) renderPaneBody(pane *widget.Pane, height int) string {
	label := ansi.ResetStyle + m.styles.PaneHeader.Render(" "+pane.Title()+" ")
	if pad := m.width - util.VisibleLen(label); pad > 0 {
		label += m.styles.PaneBorder.Render(strings.Repeat("─", pad))
	}

	rows := make([]string, 1, height-1)
	rows[0] = label
	rows = append(rows, pane.ContentRows(m.width, height-paneChromeHeight)...)
	return strings.Join(rows, "\n")
}

func (m *Model) renderPaneClosing() string {
	return ansi.ResetStyle + m.styles.PaneBorder.Render(strings.Repeat("─", m.width))
}

// layoutDock renders one dock. A pane's closing rule stays pending until the
// next rendered entry determines whether it is a real edge or a shared pane
// boundary.
func (m *Model) layoutDock(entries []ui.LayoutEntry) (string, int) {
	var parts []string
	pendingClosing := ""
	totalHeight := m.walkDock(entries, func(part dockPart) {
		if part.sharedWithPreviousPane {
			pendingClosing = ""
		} else if pendingClosing != "" {
			parts = append(parts, pendingClosing)
			pendingClosing = ""
		}

		parts = append(parts, part.view)
		pendingClosing = part.closing
	})
	if pendingClosing != "" {
		parts = append(parts, pendingClosing)
	}
	return strings.Join(parts, "\n"), totalHeight
}

// dockHeight measures a dock without rendering it. Search uses this during
// Update so viewport positioning sees the same walker and final geometry as
// View.
func (m *Model) dockHeight(entries []ui.LayoutEntry) int {
	return m.walkDock(entries, nil)
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
	}
	if m.mouseEnabled {
		view.MouseMode = tea.MouseModeCellMotion
	}
	if m.numpadMode {
		// The default kitty disambiguation flag reports NumLock-on keypad
		// digits as plain text, indistinguishable from the number row.
		// Encoding all keys as escape codes preserves the physical key;
		// associated text keeps the typed characters coming from the
		// terminal instead of being reconstructed from key codes.
		view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		view.KeyboardEnhancements.ReportAssociatedText = true
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
