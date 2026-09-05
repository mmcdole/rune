package widget

import (
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/input"
)

// inputLayout describes the picker and command field separately. Both painting
// and shared-boundary planning use these same positions, including labels.
type inputLayout struct {
	height         int
	pickerHeight   int
	results        image.Rectangle
	help           int
	body           image.Rectangle
	rules          []Rule
	header, footer int
}

func (i *Input) layout(width, height int) inputLayout {
	if height <= 0 {
		height = i.MeasureHeight(width, 1<<14)
	}
	p := inputLayout{height: height, help: -1, header: -1, footer: -1}
	if width <= 0 {
		return p
	}
	if i.PickerActive() || i.SearchActive() {
		// One editor stays at the bottom. Suggestions/results occupy the
		// rows above it; Input owns their shared separators, with no box.
		fieldHeight := min(3, max(1, height-1))
		p.pickerHeight = max(0, height-fieldHeight)
		start := 0
		if p.pickerHeight > 1 {
			p.rules = append(p.rules, Rule{At: 0, To: width})
			start = 1
		}
		p.results = image.Rect(0, start, width, p.pickerHeight)
		if i.SearchActive() && p.results.Dy() > 1 {
			p.results.Max.Y--
			p.help = p.results.Max.Y
		}
	}
	top, bottom := p.pickerHeight, height
	fieldHeight := bottom - top
	if i.composer != nil && !i.SearchActive() && (!i.PickerActive() || i.PickerInline()) {
		if fieldHeight >= 2 {
			p.header = top
			p.rules = append(p.rules, Rule{At: top, To: width})
			top++
		}
		if fieldHeight >= 3 {
			bottom--
			p.footer = bottom
			p.rules = append(p.rules, Rule{At: bottom, To: width})
		}
	} else {
		if fieldHeight >= 3 {
			p.rules = append(p.rules, Rule{At: top, To: width})
			top++
		}
		if fieldHeight >= 2 {
			bottom--
			p.rules = append(p.rules, Rule{At: bottom, To: width})
		}
	}
	p.body = image.Rect(0, top, width, bottom)
	return p
}

// Rules supplies compositor-owned decoration around View's content. Label
// styling is deferred until painting; layout itself computes only geometry.
func (i *Input) Rules(width, height int) []Rule {
	if width <= 0 || height <= 0 {
		return nil
	}
	plan := i.layout(width, height)
	if i.composer != nil && !i.SearchActive() && (!i.PickerActive() || i.PickerInline()) {
		header, toggle, footer := i.composeLabels(strings.Count(i.Value(), "\n")+1, width-4)
		for n := range plan.rules {
			rule := &plan.rules[n]
			if rule.Vertical {
				continue
			}
			switch rule.At {
			case plan.header:
				modeStyle := i.styles.InputText
				if i.SubmissionMode() == input.ModeVerbatim {
					modeStyle = i.styles.Warning
				}
				if header != "" {
					rule.Labels = append(rule.Labels, RuleLabel{Text: " " + header + " ", At: 1, Style: modeStyle})
				}
				if toggle != "" {
					rule.Labels = append(rule.Labels, RuleLabel{Text: " " + toggle + " ", At: width - 3 - ansi.StringWidth(toggle), Style: i.styles.Muted})
				}
			case plan.footer:
				hintStyle := i.styles.Muted
				if i.discardPending {
					hintStyle = i.styles.Warning
				}
				if footer != "" {
					rule.Labels = append(rule.Labels, RuleLabel{Text: " " + footer + " ", At: 1, Style: hintStyle})
				}
			}
		}
	}
	return plan.rules
}
