package widget

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
)

func TestBarAlignsUsingCompositorCellWidths(t *testing.T) {
	bar := NewBar()
	bar.SetContent(ui.BarContent{Left: "❤️", Right: "x"})
	bar.SetSize(5, 1)
	if got := bar.View(); ansi.StringWidth(got) != 5 || !strings.HasSuffix(ansi.Strip(got), "x") {
		t.Fatalf("emoji displaced right-aligned content: %q", got)
	}
}

func TestBarMeasureHeightRequiresVisibleContent(t *testing.T) {
	bar := NewBar()
	bar.SetContent(ui.BarContent{
		Left:   "\x1b[31m\x1b[0m",
		Center: "\x1b[1m\x1b[0m",
		Right:  "\x1b[4m\x1b[0m",
	})

	if got := bar.MeasureHeight(40, 10); got != 0 {
		t.Fatalf("MeasureHeight() = %d, want 0 for ANSI-only content", got)
	}

	bar.SetContent(ui.BarContent{Center: "\x1b[32mready\x1b[0m"})
	if got := bar.MeasureHeight(40, 10); got != 1 {
		t.Fatalf("MeasureHeight() = %d, want 1 for visible styled content", got)
	}
}
