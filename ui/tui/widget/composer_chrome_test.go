package widget

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/input"
)

func TestComposerLabelsPrioritizeEssentialActions(t *testing.T) {
	for _, mode := range []input.SubmissionMode{input.ModeCommand, input.ModeVerbatim} {
		t.Run(mode.String(), func(t *testing.T) {
			in := newTestInput(100)
			in.BeginCompose("first\nsecond", 0)
			in.SetSubmissionMode(mode)
			in.SetEditorAvailable(true)
			for _, width := range []int{32, 40, 60, 80, 100} {
				t.Run(fmt.Sprint(width), func(t *testing.T) {
					in.SetSize(width, 0)
					labels := inputLabels(in)
					if !strings.Contains(labels, "Enter ") || !strings.Contains(labels, "Ctrl+J newline") {
						t.Fatalf("essential editing hints missing at width %d: %q", width, labels)
					}
					destination := "Alt+V verbatim"
					if mode == input.ModeVerbatim {
						destination = "Alt+V command"
					}
					if !strings.Contains(labels, destination) {
						t.Fatalf("mode switch missing: %q", labels)
					}
					if width >= 40 && !strings.Contains(labels, "2 lines") {
						t.Fatalf("line count missing: %q", labels)
					}
					if width == 32 && strings.Contains(labels, "2 lines") {
						t.Fatalf("line count crowded out mode switch: %q", labels)
					}
					if width == 100 && !strings.Contains(labels, "Ctrl+E editor") {
						t.Fatalf("available editor hint missing: %q", labels)
					}
					if width == 32 && (strings.Contains(labels, "Ctrl+E") || strings.Contains(labels, "Alt+Enter")) {
						t.Fatalf("secondary hints crowded essential actions: %q", labels)
					}
				})
			}
		})
	}
}

func TestComposerLabelsStayCompleteAndInsideTheirRules(t *testing.T) {
	in := newTestInput(100)
	in.BeginCompose("first\nsecond", 0)
	in.SetEditorAvailable(true)
	complete := map[string]bool{
		"COMMAND": true, "VERBATIM": true, "COMMAND · 2 lines": true, "VERBATIM · 2 lines": true,
		"Alt+V command": true, "Alt+V verbatim": true,
		"Enter run": true, "Enter send": true, "Ctrl+J newline": true,
		"Alt+Enter run": true, "Esc×2 discard": true, "Ctrl+E editor": true,
		"Esc again to discard": true, "Esc to discard": true,
	}
	for _, mode := range []input.SubmissionMode{input.ModeCommand, input.ModeVerbatim} {
		in.SetSubmissionMode(mode)
		for _, confirmation := range []bool{false, true} {
			in.discardPending = confirmation
			for width := 1; width <= 120; width++ {
				for _, rule := range in.Rules(width, in.MeasureHeight(width, 100)) {
					end := 0
					for _, label := range rule.Labels {
						if label.At <= end || label.At+ansi.StringWidth(label.Text) >= width {
							t.Fatalf("overlapping/outside label at width %d: %+v", width, rule)
						}
						end = label.At + ansi.StringWidth(label.Text)
						value := strings.TrimSpace(label.Text)
						if complete[value] {
							continue
						}
						for _, hint := range strings.Split(value, " · ") {
							if !complete[hint] {
								t.Fatalf("incomplete hint at width %d: %q", width, hint)
							}
						}
					}
					// Positioning a copy in the frame must not move the widget's own labels.
					translated := rule.Translate(image.Pt(7, 3))
					for n, label := range rule.Labels {
						if translated.Labels[n].At != label.At+7 {
							t.Fatal("label translation mutated local coordinates")
						}
					}
				}
			}
		}
	}
}
