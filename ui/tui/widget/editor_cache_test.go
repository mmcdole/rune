package widget

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestEditorLayoutTracksEditsAfterMeasurement(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*editor)
		want string
	}{
		{"replace_same_length", func(c *editor) { c.Set("XYZ\n123", 3) }, "XYZ\n123"},
		{"insert", func(c *editor) { c.Insert("!") }, "abc!\ndef"},
		{"backspace", (*editor).Backspace, "ab\ndef"},
		{"delete", (*editor).Delete, "abcdef"},
		{"delete_word", (*editor).DeleteWordBack, "\ndef"},
		{"delete_start", (*editor).DeleteToLineStart, "\ndef"},
		{"delete_end", (*editor).DeleteToLineEnd, "abcdef"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newEditor("abc\ndef", 3)
			c.layout(80) // measurement before the edit must not freeze its pixels
			tc.edit(c)
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

func TestEditorKeepsRowsDuringNavigationAndMeasurement(t *testing.T) {
	for _, value := range []string{"a\tb界é\n", strings.Repeat("long draft\n", 1000), strings.Repeat("wrap ", 1000)} {
		c := newEditor(value, 0)
		rows := c.layout(20).rows
		for _, width := range []int{1, 10, 80, 270} {
			c.measureRows(width)
			if &c.layout(20).rows[0] != &rows[0] {
				t.Fatal("measurement evicted the editing layout")
			}
		}
		// Check every source insertion point, including grapheme interiors and
		// empty continuation rows created by tabs at narrow widths.
		for rowIndex, row := range rows {
			for _, point := range row.points {
				c.SetCursor(point.offset)
				layout := c.layout(20)
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

func TestEditorMeasurementMatchesFullLayout(t *testing.T) {
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
			c := newEditor(value, 0)
			for width := 1; width <= 80; width++ {
				got := c.measureRows(width)
				want := min(len(c.layout(width).rows), maxEditorBodyRows)
				if got != want {
					t.Fatalf("width %d, %q: measured %d, want %d", width, value, got, want)
				}
			}
		})
	}
}
