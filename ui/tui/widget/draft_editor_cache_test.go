package widget

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestDraftEditorLayoutTracksEditsAfterMeasurement(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*draftEditor)
		want string
	}{
		{"replace_same_length", func(c *draftEditor) { c.Set("XYZ\n123", 3) }, "XYZ\n123"},
		{"insert", func(c *draftEditor) { c.Insert("!") }, "abc!\ndef"},
		{"backspace", (*draftEditor).Backspace, "ab\ndef"},
		{"delete", (*draftEditor).Delete, "abcdef"},
		{"delete_word", (*draftEditor).DeleteWordBack, "\ndef"},
		{"delete_start", (*draftEditor).DeleteToLineStart, "\ndef"},
		{"delete_end", (*draftEditor).DeleteToLineEnd, "abcdef"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newDraftEditor("abc\ndef", 3)
			c.layout(80) // measurement before the edit must not freeze its pixels
			tc.edit(c)
			if got, want := c.lines(), strings.Count(tc.want, "\n")+1; got != want {
				t.Fatalf("line count = %d, want %d", got, want)
			}
			var rows []string
			for _, row := range c.layout(80).rows {
				var text strings.Builder
				for _, glyph := range row.glyphs {
					text.WriteString(glyph.text)
				}
				rows = append(rows, text.String())
			}
			if got := strings.Join(rows, "\n"); got != tc.want {
				t.Fatalf("rendered draft = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDraftEditorKeepsRowsDuringNavigationAndMeasurement(t *testing.T) {
	for _, tc := range []struct {
		value string
		width int
	}{
		{"a\tb界é\n", 1}, {"a\tb界é\n", 20},
		{strings.Repeat("long draft\n", 1000), 20}, {strings.Repeat("wrap ", 1000), 20},
	} {
		c := newDraftEditor(tc.value, 0)
		rows := c.layout(tc.width).rows
		for _, width := range []int{1, 10, 80, 270} {
			c.measureRows(width)
			if &c.layout(tc.width).rows[0] != &rows[0] {
				t.Fatal("measurement evicted the editing layout")
			}
		}
		// Check every source insertion point, including grapheme interiors and
		// empty continuation rows created by tabs at narrow widths.
		for rowIndex, row := range rows {
			for _, point := range row.points {
				c.SetCursor(point.offset)
				layout := c.layout(tc.width)
				if &layout.rows[0] != &rows[0] {
					t.Fatal("cursor movement reshaped the draft")
				}
				if layout.cursorRow != rowIndex || layout.cursorCol != point.col {
					t.Fatalf("offset %d: cursor = (%d,%d), want (%d,%d)", point.offset, layout.cursorRow, layout.cursorCol, rowIndex, point.col)
				}
			}
		}
	}
}

func TestDraftEditorMeasurementMatchesFullLayout(t *testing.T) {
	values := []string{"", "abcdef", "a\tb界é\n", strings.Repeat("\t", 20), strings.Repeat("long line 界 é ", 100), strings.Repeat("x\n", 10)}
	random := rand.New(rand.NewSource(42))
	alphabet := []rune("ab界\t\ń👩‍💻")
	for range 30 {
		var value strings.Builder
		for range random.Intn(200) {
			value.WriteRune(alphabet[random.Intn(len(alphabet))])
		}
		values = append(values, value.String())
	}
	for n, value := range values {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			c := newDraftEditor(value, 0)
			for width := 1; width <= 80; width++ {
				got := c.measureRows(width)
				want := min(len(c.layout(width).rows), maxDraftBodyRows)
				if got != want {
					t.Fatalf("width %d, %q: measured %d, want %d", width, value, got, want)
				}
			}
		})
	}
}
