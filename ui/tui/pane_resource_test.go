package tui

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
)

func TestPaneRegistryAppliesLifecycleToReservedOutput(t *testing.T) {
	output := newOutputController(style.DefaultStyles())
	panes := newPaneRegistry(output)

	reserved, ok := panes.Lookup(ui.OutputPaneName)
	if !ok || reserved != output {
		t.Fatalf("reserved output = (%v, %v), want pre-created controller", reserved, ok)
	}
	if created := panes.Create(ui.OutputPaneName); created != output {
		t.Fatal("creating output replaced the reserved resource")
	}

	chat := panes.Create("chat")
	if again := panes.Create("chat"); again != chat {
		t.Fatal("re-creating a pane replaced the existing buffer")
	}
	if _, ok := panes.Lookup("missing"); ok {
		t.Fatal("unknown pane exists without create or write")
	}

	written := panes.Write("log", "one\ntwo")
	written.SetSize(20, 2)
	if got := written.View(); got != "one\ntwo" {
		t.Fatalf("written pane view = %q, want two stored rows", got)
	}
}

func TestOutputRetainsLastPlacementGeometryWhileHidden(t *testing.T) {
	output := newOutputController(style.DefaultStyles())
	output.setFallbackGeometry(80, 24)
	output.setFallbackGeometry(100, 30)
	if output.wrapWidth != 100 {
		t.Fatalf("unplaced fallback width = %d, want latest terminal width 100", output.wrapWidth)
	}

	output.setGeometry(24, 6)
	output.setFallbackGeometry(120, 40)
	if output.wrapWidth != 24 {
		t.Fatalf("hidden output width = %d, want retained placement width 24", output.wrapWidth)
	}
	output.Write(strings.Repeat("x", 30))
	if output.buffer.Count() != 2 {
		t.Fatalf("hidden write produced %d rows at retained width, want 2", output.buffer.Count())
	}
}
