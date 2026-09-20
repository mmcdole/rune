package tui

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/widget"
)

func TestPaneLookupPreservesOutputAndContent(t *testing.T) {
	m := NewModel(nil)
	output := m.output

	reserved, ok := m.panes[ui.OutputPaneName]
	if !ok || reserved != output {
		t.Fatalf("reserved output = (%v, %v), want original Output", reserved, ok)
	}
	if created := m.pane(ui.OutputPaneName); created != output {
		t.Fatal("creating output replaced the Output widget")
	}

	chat := m.pane("chat")
	if again := m.pane("chat"); again != chat {
		t.Fatal("re-creating a pane replaced the existing buffer")
	}
	if _, ok := m.panes["missing"]; ok {
		t.Fatal("unknown pane exists without create or write")
	}

	written := m.pane("log")
	written.Write("one\ntwo")
	written.SetSize(20, 2)
	if got := written.View(); got != "one\ntwo" {
		t.Fatalf("written pane view = %q, want two stored rows", got)
	}
}

func TestOutputRetainsLastPlacementGeometryWhileHidden(t *testing.T) {
	output := widget.NewOutput(100000, style.DefaultStyles())
	output.SetFallbackSize(80, 24)
	output.SetFallbackSize(100, 30)
	if output.Width() != 100 {
		t.Fatalf("unplaced fallback width = %d, want latest terminal width 100", output.Width())
	}

	output.SetSize(24, 6)
	output.SetFallbackSize(120, 40)
	if output.Width() != 24 {
		t.Fatalf("hidden output width = %d, want retained placement width 24", output.Width())
	}
	output.Write(strings.Repeat("x", 30))
	if output.Scrollback().Count() != 2 {
		t.Fatalf("hidden write produced %d rows at retained width, want 2", output.Scrollback().Count())
	}
}
