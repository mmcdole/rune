package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/util"
)

func addVisiblePane(t *testing.T, m *Model, name string, lines ...string) {
	t.Helper()
	m.panes.Create(name)
	pane, ok := m.panes.Lookup(name)
	if !ok {
		t.Fatalf("pane %q was not created", name)
	}
	for _, line := range lines {
		pane.Write(line)
	}
	pane.SetVisible(true)
}

func plainDockRows(view string) []string {
	if view == "" {
		return nil
	}
	return strings.Split(runetext.StripANSI(view), "\n")
}

func countRowsEqual(rows []string, want string) int {
	count := 0
	for _, row := range rows {
		if row == want {
			count++
		}
	}
	return count
}

func TestPaneCompositorRendersStandaloneFrame(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha", "alpha one", "alpha two")
	entries := []ui.LayoutEntry{{Name: "alpha", Height: 4}}

	view, height := m.layoutDock(entries)
	rows := plainDockRows(view)
	if height != 4 || len(rows) != 4 {
		t.Fatalf("standalone geometry = (%d measured, %d rendered), want 4", height, len(rows))
	}
	if got := m.dockHeight(entries); got != height {
		t.Fatalf("dockHeight = %d, want rendered height %d", got, height)
	}
	if !strings.Contains(rows[0], " alpha ") {
		t.Fatalf("header = %q, want alpha title", rows[0])
	}
	if rows[1] != "alpha one" || rows[2] != "alpha two" {
		t.Fatalf("content rows = %q, want alpha lines", rows[1:3])
	}
	closing := strings.Repeat("─", m.width)
	if rows[3] != closing {
		t.Fatalf("closing row = %q, want full-width rule", rows[3])
	}
}

func TestPaneCompositorSharesAdjacentBoundaries(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha", "a1", "a2")
	addVisiblePane(t, m, "bravo", "b1", "b2", "b3", "b4")
	addVisiblePane(t, m, "charlie", "c1", "c2")
	bravo, _ := m.panes.Lookup("bravo")
	bravo.ScrollUp(1)
	bravo.Write("b5")

	entries := []ui.LayoutEntry{
		{Name: "alpha", Height: 4},
		{Name: "bravo", Height: 4},
		{Name: "charlie", Height: 4},
	}
	view, height := m.layoutDock(entries)
	rows := plainDockRows(view)
	if height != 10 || len(rows) != 10 {
		t.Fatalf("three-pane geometry = (%d measured, %d rendered), want 10", height, len(rows))
	}
	if got := m.dockHeight(entries); got != height {
		t.Fatalf("dockHeight = %d, want rendered height %d", got, height)
	}
	if !strings.Contains(rows[3], " bravo · scroll +1 ") {
		t.Fatalf("first shared boundary = %q, want bravo new-line indicator", rows[3])
	}
	if !strings.Contains(rows[6], " charlie ") {
		t.Fatalf("second shared boundary = %q, want charlie title", rows[6])
	}
	closing := strings.Repeat("─", m.width)
	if got := countRowsEqual(rows, closing); got != 1 {
		t.Fatalf("pure closing rules = %d, want only the final pane rule", got)
	}
}

func TestPaneCompositorSnapshotsRepeatedPaneGeometry(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "shared", "line 1", "line 2", "line 3", "line 4", "line 5")
	entries := []ui.LayoutEntry{
		{Name: "shared", Height: 4},
		{Name: "shared", Height: 5},
	}

	view, height := m.layoutDock(entries)
	rows := plainDockRows(view)
	if height != 8 || len(rows) != 8 {
		t.Fatalf("repeated-pane geometry = (%d measured, %d rendered), want 8", height, len(rows))
	}
	if rows[1] != "line 4" || rows[2] != "line 5" {
		t.Fatalf("first occurrence content = %q, want two-row tail", rows[1:3])
	}
	if !strings.Contains(rows[3], " shared ") {
		t.Fatalf("shared boundary = %q, want second title", rows[3])
	}
	if got := rows[4:7]; strings.Join(got, "|") != "line 3|line 4|line 5" {
		t.Fatalf("second occurrence content = %q, want three-row tail", got)
	}
}

func TestPaneCompositorUsesRenderedAdjacency(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha", "a1", "a2")
	addVisiblePane(t, m, "charlie", "c1", "c2")
	m.panes.Create("hidden")
	m.syncBars(map[string]ui.BarContent{
		"empty":  {},
		"middle": {Left: "middle bar"},
	})

	t.Run("skipped entries preserve adjacency", func(t *testing.T) {
		entries := []ui.LayoutEntry{
			{Name: "alpha", Height: 4},
			{Name: "missing"},
			{Name: "hidden", Height: 4},
			{Name: "empty"},
			{Name: "charlie", Height: 4},
		}
		view, height := m.layoutDock(entries)
		rows := plainDockRows(view)
		if height != 7 || len(rows) != 7 {
			t.Fatalf("skipped-entry geometry = (%d measured, %d rendered), want 7", height, len(rows))
		}
		if !strings.Contains(rows[3], " charlie ") {
			t.Fatalf("shared boundary = %q, want charlie title", rows[3])
		}
	})

	tests := []struct {
		name   string
		middle ui.LayoutEntry
	}{
		{name: "separator", middle: ui.LayoutEntry{Name: "separator"}},
		{name: "visible bar", middle: ui.LayoutEntry{Name: "middle"}},
		{name: "input", middle: ui.LayoutEntry{Name: "input"}},
	}
	for _, test := range tests {
		t.Run(test.name+" breaks adjacency", func(t *testing.T) {
			entries := []ui.LayoutEntry{
				{Name: "alpha", Height: 4},
				test.middle,
				{Name: "charlie", Height: 4},
			}
			view, height := m.layoutDock(entries)
			rows := plainDockRows(view)
			if got := m.dockHeight(entries); got != height || len(rows) != height {
				t.Fatalf("geometry = dock %d, measured %d, rendered %d", got, height, len(rows))
			}
			closing := strings.Repeat("─", m.width)
			if got := countRowsEqual(rows, closing); got < 2 {
				t.Fatalf("pure pane closing rules = %d, want a rule on both sides of rendered middle entry", got)
			}
		})
	}

	t.Run("rendered bar sits between pane frames", func(t *testing.T) {
		entries := []ui.LayoutEntry{
			{Name: "alpha", Height: 4},
			{Name: "middle"},
			{Name: "charlie", Height: 4},
		}
		view, _ := m.layoutDock(entries)
		rows := plainDockRows(view)
		closing := strings.Repeat("─", m.width)
		if rows[3] != closing || !strings.Contains(rows[4], "middle bar") || !strings.Contains(rows[5], " charlie ") {
			t.Fatalf("boundary rows = %q, want pane footer, bar, then pane header", rows[3:6])
		}
	})
}

func TestPaneCompositorRecomputesAfterVisibilityChanges(t *testing.T) {
	m := newBareModel(t)
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		addVisiblePane(t, m, name, name+" content")
	}
	entries := []ui.LayoutEntry{
		{Name: "alpha", Height: 3},
		{Name: "bravo", Height: 3},
		{Name: "charlie", Height: 3},
	}

	assertHeight := func(want int) {
		t.Helper()
		view, got := m.layoutDock(entries)
		if rows := plainDockRows(view); got != want || len(rows) != want || m.dockHeight(entries) != want {
			t.Fatalf("geometry = (%d measured, %d rendered, %d dock), want %d", got, len(rows), m.dockHeight(entries), want)
		}
		if rules := countRowsEqual(plainDockRows(view), strings.Repeat("─", m.width)); rules != 1 {
			t.Fatalf("closing rules = %d, want one final rule", rules)
		}
	}

	assertHeight(7)
	m.panes.SetVisible("bravo", false)
	assertHeight(5)
	m.panes.SetVisible("charlie", false)
	assertHeight(3)
}

func TestPaneCompositorIntrinsicAndMinimumHeights(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha")
	addVisiblePane(t, m, "bravo")

	t.Run("intrinsic panes retain ten content rows", func(t *testing.T) {
		entries := []ui.LayoutEntry{{Name: "alpha"}, {Name: "bravo"}}
		view, height := m.layoutDock(entries)
		rows := plainDockRows(view)
		if height != 23 || len(rows) != 23 || m.dockHeight(entries) != 23 {
			t.Fatalf("intrinsic geometry = (%d measured, %d rendered, %d dock), want 23", height, len(rows), m.dockHeight(entries))
		}
		if !strings.Contains(rows[0], " alpha ") || !strings.Contains(rows[11], " bravo ") {
			t.Fatalf("intrinsic headers at rows 0 and 11 = %q, %q", rows[0], rows[11])
		}
	})

	t.Run("height below chrome clamps to two", func(t *testing.T) {
		entries := []ui.LayoutEntry{{Name: "alpha", Height: 1}}
		view, height := m.layoutDock(entries)
		rows := plainDockRows(view)
		if height != 2 || len(rows) != 2 || m.dockHeight(entries) != 2 {
			t.Fatalf("minimum geometry = (%d measured, %d rendered, %d dock), want 2", height, len(rows), m.dockHeight(entries))
		}
		if !strings.Contains(rows[0], " alpha ") || rows[1] != strings.Repeat("─", m.width) {
			t.Fatalf("minimum pane rows = %q, want header and closing rule", rows)
		}
	})
}

func TestPaneCompositorKeepsDockEdgesIndependent(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "top", "top one", "top two")
	addVisiblePane(t, m, "bottom", "bottom one", "bottom two")

	next, _ := m.Update(ui.UpdateLayoutMsg{
		Top:    []ui.LayoutEntry{{Name: "top", Height: 4}},
		Bottom: []ui.LayoutEntry{{Name: "bottom", Height: 4}},
	})
	m = next.(*Model)
	rows := plainDockRows(m.View().Content)
	if len(rows) != m.height {
		t.Fatalf("full frame has %d rows, want terminal height %d", len(rows), m.height)
	}
	closing := strings.Repeat("─", m.width)
	if rows[3] != closing {
		t.Fatalf("top dock edge = %q, want closing rule directly above viewport", rows[3])
	}
	if rows[len(rows)-1] != closing {
		t.Fatalf("bottom dock edge = %q, want final closing rule", rows[len(rows)-1])
	}
	if got := countRowsEqual(rows, closing); got != 2 {
		t.Fatalf("independent dock closing rules = %d, want 2", got)
	}
}

func TestPaneCompositorReturnsSharedRowsToViewport(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha", "a1", "a2")
	addVisiblePane(t, m, "bravo", "b1", "b2")

	next, _ := m.Update(ui.UpdateLayoutMsg{
		Top: []ui.LayoutEntry{
			{Name: "alpha", Height: 4},
			{Name: "bravo", Height: 4},
		},
	})
	m = next.(*Model)
	rows := plainDockRows(m.View().Content)
	if len(rows) != m.height {
		t.Fatalf("full frame has %d rows, want terminal height %d", len(rows), m.height)
	}
	if got, want := m.viewport.PreferredHeight(), m.height-7; got != want {
		t.Fatalf("viewport height = %d, want %d after reclaiming shared row", got, want)
	}
	closing := strings.Repeat("─", m.width)
	if rows[6] != closing {
		t.Fatalf("top dock edge = %q, want final rule directly above viewport", rows[6])
	}
}

func TestPaneCompositorHeaderWidths(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha")
	view, _ := m.layoutDock([]ui.LayoutEntry{{Name: "alpha", Height: 2}})
	for i, row := range strings.Split(view, "\n") {
		if width := util.VisibleLen(row); width != m.width {
			t.Fatalf("frame row %d width = %d, want %d", i, width, m.width)
		}
	}
}

func TestPaneCompositorResetsStylesBeforeChrome(t *testing.T) {
	m := newBareModel(t)
	addVisiblePane(t, m, "alpha", "\x1b[7munterminated reverse")
	addVisiblePane(t, m, "bravo")

	view, _ := m.layoutDock([]ui.LayoutEntry{
		{Name: "alpha", Height: 3},
		{Name: "bravo", Height: 2},
	})
	if !strings.Contains(view, "\x1b[7munterminated reverse\n"+ansi.ResetStyle) {
		t.Fatalf("shared pane header does not reset preceding content style: %q", view)
	}
	if !strings.HasPrefix(m.renderPaneClosing(), ansi.ResetStyle) {
		t.Fatalf("pane closing rule does not begin with a style reset: %q", m.renderPaneClosing())
	}
}
