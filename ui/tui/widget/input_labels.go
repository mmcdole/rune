package widget

import (
	"fmt"
	"image"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/input"
)

// Label places styled text over a horizontal rule. Position is relative to
// the widget's allocated rectangle; the renderer translates and clips it.
type Label struct {
	Position image.Point
	Text     string
}

// Labels supplies current styled text at the applied size. It uses the same
// geometry as RuleRows and View without changing placement or navigation state.
func (i *Input) Labels() []Label {
	plan := i.layout(i.width, i.height)
	if plan.header < 0 && plan.footer < 0 {
		return nil
	}
	header, toggle, footer := i.draftLabels(i.draftEditor.lines(), i.width-4)
	var labels []Label
	if plan.header >= 0 {
		modeStyle := i.styles.InputText
		if i.SubmissionMode() == input.ModeVerbatim {
			modeStyle = i.styles.Warning
		}
		if header != "" {
			labels = append(labels, Label{Position: image.Pt(1, plan.header), Text: modeStyle.Render(" " + header + " ")})
		}
		if toggle != "" {
			labels = append(labels, Label{
				Position: image.Pt(i.width-3-ansi.StringWidth(toggle), plan.header),
				Text:     i.styles.Muted.Render(" " + toggle + " "),
			})
		}
	}
	if plan.footer >= 0 && footer != "" {
		hintStyle := i.styles.Muted
		if i.discardPending {
			hintStyle = i.styles.Warning
		}
		labels = append(labels, Label{Position: image.Pt(1, plan.footer), Text: hintStyle.Render(" " + footer + " ")})
	}
	return labels
}

// draftLabels fits complete labels in the available cells. Mode switching
// takes precedence over line count; submit and newline precede secondary actions.
func (i *Input) draftLabels(lines, width int) (header, toggle, footer string) {
	mode, destination, submit := "COMMAND", "verbatim", i.actionHint("submit", "run")
	if i.SubmissionMode() == input.ModeVerbatim {
		mode, destination, submit = "VERBATIM", "command", i.actionHint("submit", "send")
	}
	word := "lines"
	if lines == 1 {
		word = "line"
	}
	title := fmt.Sprintf("%s · %d %s", mode, lines, word)
	toggle = i.actionHint("toggle_mode", destination)
	header = title
	if ansi.StringWidth(header)+3+ansi.StringWidth(toggle) > width {
		header = mode
	}
	if ansi.StringWidth(header)+3+ansi.StringWidth(toggle) > width {
		toggle = ""
		header = fitDraftHints(width, title)
		if header == "" {
			header = fitDraftHints(width, mode)
		}
	}
	hints := []string{submit, i.actionHint("newline", "newline")}
	cancel := i.keys.Hint("cancel")
	if cancel != "" {
		hints = append(hints, cancel+"×2 discard")
	}
	hints = append(hints, i.actionHint("open_editor", "editor"))
	footer = fitDraftHints(width, hints...)
	if i.discardPending && cancel != "" {
		footer = fitDraftHints(width, cancel+" again to discard")
		if footer == "" {
			footer = fitDraftHints(width, cancel+" to discard")
		}
	}

	return header, toggle, footer
}

// fitDraftHints keeps hints in priority order without cutting a key or label.
func fitDraftHints(width int, hints ...string) string {
	var fitted string
	for _, hint := range hints {
		if hint == "" {
			continue
		}
		candidate := hint
		if fitted != "" {
			candidate = fitted + " · " + hint
		}
		if ansi.StringWidth(candidate) > width {
			break
		}
		fitted = candidate
	}
	return fitted
}

func (i *Input) actionHint(action, label string) string {
	key := i.keys.Hint(action)
	if key == "" {
		return ""
	}
	return key + " " + label
}
