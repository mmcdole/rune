package widget

import (
	"testing"

	"github.com/mmcdole/rune/ui"
)

func TestBarPreferredHeightRequiresVisibleContent(t *testing.T) {
	bar := NewBar("status")
	bar.SetContent(ui.BarContent{
		Left:   "\x1b[31m\x1b[0m",
		Center: "\x1b[1m\x1b[0m",
		Right:  "\x1b[4m\x1b[0m",
	})

	if got := bar.PreferredHeight(); got != 0 {
		t.Fatalf("PreferredHeight() = %d, want 0 for ANSI-only content", got)
	}

	bar.SetContent(ui.BarContent{Center: "\x1b[32mready\x1b[0m"})
	if got := bar.PreferredHeight(); got != 1 {
		t.Fatalf("PreferredHeight() = %d, want 1 for visible styled content", got)
	}
}
