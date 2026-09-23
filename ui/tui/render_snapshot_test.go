package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/mmcdole/rune/ui"
)

var updateRenderSnapshots = flag.Bool("update-render", false, "rewrite reviewed render snapshots")

// Replay onto the real Model, then normalize through cells before comparing.
// Equivalent ANSI encodings pass; changed glyphs, colors, cursor styles, and
// geometry do not. Each quoted line is one terminal row, including its padding.
func TestRenderSnapshots(t *testing.T) {
	for _, scene := range []string{"normal", "draft_editor", "picker", "search", "scrolled"} {
		t.Run(scene, func(t *testing.T) {
			m := renderFixture(80, 24, true)
			m.Update(printLineMsg("\x1b[35mUnicode:\x1b[0m 界 e\u0301 👩‍💻 1️⃣\tend"))
			m.Update(setInputMsg("look north"))
			switch scene {
			case "draft_editor":
				m.Update(setInputMsg("say hello\n\t界 e\u0301 👩‍💻\nlook"))
			case "picker":
				m.Update(showPickerMsg{options: ui.PickerOptions{Title: "History", Items: []ui.PickerItem{{Text: "look north"}, {Text: "say hello"}}}})
			case "search":
				m.Update(showSearchMsg{options: ui.SearchOptions{Query: "dragon"}})
			case "scrolled":
				m.Update(paneScrollUpMsg{Name: ui.OutputPaneName, Lines: 10})
				m.Update(printLineMsg("new output while reading history"))
			}
			view := m.View().Content
			assertExactBlock(t, view, 80, 24)
			canvas := newCanvas(80, 24)
			uv.NewStyledString(view).Draw(canvas, canvas.Bounds())
			var snapshot strings.Builder
			for _, row := range canvas.Lines {
				snapshot.WriteString(strconv.Quote(row.Render()))
				snapshot.WriteByte('\n')
			}
			path := filepath.Join("testdata", "render", scene+".golden")
			if *updateRenderSnapshots {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(snapshot.String()), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.String() != string(want) {
				t.Fatalf("screen differs from %s; inspect the change before using -update-render\nwant:\n%s\ngot:\n%s", path, want, snapshot.String())
			}
		})
	}
}
