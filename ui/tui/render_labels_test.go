package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

func TestInputLabelsRefreshWithoutLayout(t *testing.T) {
	for _, width := range []int{120, 40, 20, 3} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := resizeModel(t, NewModel(make(chan ui.UIEvent, 100)), width, 12)
			m.input.OpenDraftEditor("first\nsecond", 0)
			// Place input away from the origin and between shared borders.
			// Its fixed height keeps geometry unchanged throughout the edits.
			setLayout(m, containedInputLayout(ui.PaneBorderFull, ui.Cells(5), 0))
			m.render()

			for _, step := range []struct {
				name   string
				change func()
				want   string
				absent string
			}{
				{"mode", func() { m.input.SetSubmissionMode(input.ModeCommand) }, "COMMAND", "VERBATIM"},
				{"discard", func() { m.input.ConfirmDiscard() }, "Esc again to discard", "Esc×2 discard"},
				{"continue", m.input.ContinueEditing, "Esc×2 discard", "Esc again to discard"},
				{"line_count", func() { m.input.SetValue("first\nsecond\nthird") }, "3 lines", "2 lines"},
				{"remove_hints", func() { m.input.SetBindings(nil) }, "COMMAND", "Enter run"},
				{"remove_labels", m.input.Reset, "", "COMMAND"},
			} {
				t.Run(step.name, func(t *testing.T) {
					step.change()
					// Repaint directly: no dispatch, layout, sizing, or canvas clear.
					m.render()
					got := m.screen
					assertExactBlock(t, got, width, 12)
					if width == 120 {
						plain := ansi.Strip(got)
						if !strings.Contains(plain, step.want) || strings.Contains(plain, step.absent) {
							t.Fatalf("want %q without %q:\n%s", step.want, step.absent, plain)
						}
					}
					// Reusing the plan and canvas must match a fresh layout/render,
					// including colors, erased label tails, and shared junctions.
					m.applyLayout()
					m.render()
					if got != m.screen {
						t.Fatalf("reused layout left stale or misplaced labels\nreused:\n%s\nfresh:\n%s", got, m.screen)
					}
				})
			}
		})
	}
}
