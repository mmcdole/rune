package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

// normalized returns tree with production's axis-aware size defaults, as
// Session delivers it. Test fixtures are always valid, so an error panics.
func normalized(tree ui.LayoutTree) ui.LayoutTree {
	normal, err := ui.NormalizeLayoutTree(tree)
	if err != nil {
		panic(fmt.Sprintf("test layout is invalid: %v", err))
	}
	return normal
}

// layoutUpdate is the updateLayoutMsg Session would send for tree.
func layoutUpdate(tree ui.LayoutTree) updateLayoutMsg {
	return updateLayoutMsg(normalized(tree))
}

// newTestModel builds a model with a sized window and enough
// scrollback to scroll.
func newTestModel(t *testing.T) *Model {
	t.Helper()

	events := make(chan ui.UIEvent, 256)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(*Model)

	for i := 0; i < 100; i++ {
		next, _ = m.Update(printLineMsg(fmt.Sprintf("line %d", i)))
		m = next.(*Model)
	}
	return m
}

// newBareModel builds a sized model with an empty scrollback, for
// tests that assert on exact line counts and ordering.
func newBareModel(t *testing.T) *Model {
	t.Helper()

	events := make(chan ui.UIEvent, 64)
	m := NewModel(events)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func wantScrollback(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := m.output.Scrollback().Count(); got != len(want) {
		t.Fatalf("scrollback has %d lines, want %d", got, len(want))
	}
	for i, w := range want {
		if got := m.output.Scrollback().At(i); got != w {
			t.Fatalf("scrollback[%d] = %q, want %q", i, got, w)
		}
	}
}

// wantOutputRows asserts the rendered output window, ignoring the blank
// rows that pad the top of a short scrollback.
func wantOutputRows(t *testing.T, m *Model, want ...string) {
	t.Helper()
	rows := strings.Split(m.output.View(), "\n")
	for len(rows) > 0 && rows[0] == "" {
		rows = rows[1:]
	}
	if len(rows) != len(want) {
		t.Fatalf("output shows %d rows %q, want %q", len(rows), rows, want)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Fatalf("output row %d = %q, want %q (rows %q)", i, rows[i], w, rows)
		}
	}
}
