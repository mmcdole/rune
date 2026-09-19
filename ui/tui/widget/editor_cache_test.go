package widget

import (
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
