package widget

import (
	"testing"

	"github.com/mmcdole/rune/ui"
)

func TestBarAlignsUsingRendererCellWidths(t *testing.T) {
	tests := []struct {
		name    string
		content ui.BarContent
		width   int
		want    string
	}{
		{"two sections", ui.BarContent{Left: "L", Right: "R"}, 9, "L       R"},
		{"emoji", ui.BarContent{Left: "❤️", Right: "x"}, 5, "❤️  x"},
		{"centered", ui.BarContent{Left: "L", Center: "C", Right: "R"}, 9, "L   C   R"},
		{"even width", ui.BarContent{Left: "L", Center: "C", Right: "R"}, 10, "L   C    R"},
		{"center only", ui.BarContent{Center: "C"}, 9, "    C    "},
		{"wide sections", ui.BarContent{Left: "❤️", Center: "界", Right: "R"}, 10, "❤️  界   R"},
		{"styled center", ui.BarContent{Left: "L", Center: "\x1b[32mC\x1b[0m", Right: "R"}, 9, "L   \x1b[32mC\x1b[0m   R"},
		{"invisible center", ui.BarContent{Left: "L", Center: "\x1b[31m", Right: "R"}, 9, "L       R"},
		{"narrow centered", ui.BarContent{Left: "L", Center: "C", Right: "R"}, 4, "L C "},
		{"crowded center", ui.BarContent{Left: "long", Center: "C", Right: "R"}, 6, "long C"},
		{"narrow two sections", ui.BarContent{Left: "L", Right: "R"}, 2, "L "},
		{"wide clipping", ui.BarContent{Left: "界", Center: "C", Right: "R"}, 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bar Bar
			bar.SetContent(tt.content)
			bar.SetSize(tt.width, 1)
			if got := bar.View(); got != tt.want {
				t.Fatalf("View() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBarMeasureHeightRequiresVisibleContent(t *testing.T) {
	var bar Bar
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
