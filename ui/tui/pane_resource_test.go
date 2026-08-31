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
	if got := written.View(20, 2); got != "one\ntwo" {
		t.Fatalf("written pane view = %q, want two stored rows", got)
	}
}

func TestOutputClearInvalidatesOldBatchAndPreservesPrompt(t *testing.T) {
	output := newOutputController(style.DefaultStyles())
	output.setFallbackGeometry(20, 4)
	output.setPrompt("HP> ")

	oldGeneration, scheduled := output.printServer("old one")
	if !scheduled {
		t.Fatal("first old row did not schedule a batch tick")
	}
	output.printServer("old two")
	output.Clear()

	newGeneration, scheduled := output.printServer("new one")
	if !scheduled || newGeneration == oldGeneration {
		t.Fatal("post-clear output did not start a new batch generation")
	}
	output.printServer("new two")

	if output.tick(oldGeneration) {
		t.Fatal("stale tick was not ignored")
	}
	if len(output.pendingRows) != 1 || output.pendingRows[0] != "new two" {
		t.Fatalf("stale tick disturbed new pending rows: %q", output.pendingRows)
	}
	if !output.tick(newGeneration) {
		t.Fatal("current tick did not flush and rearm")
	}
	if output.buffer.Count() != 2 || output.buffer.At(0) != "new one" || output.buffer.At(1) != "new two" {
		t.Fatalf("post-clear buffer = [%q, %q], want only new rows", output.buffer.At(0), output.buffer.At(1))
	}
	if got := output.View(20, 4); !strings.HasSuffix(got, "HP> ") {
		t.Fatalf("clear removed live prompt: %q", got)
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
