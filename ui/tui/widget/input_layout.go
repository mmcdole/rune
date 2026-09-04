package widget

import (
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// inputLayout describes the picker and command field separately. Both painting
// and shared-boundary planning use these same positions, including labels.
type inputLayout struct {
	height         int
	pickerHeight   int
	body           image.Rectangle
	rules          []Rule
	header, footer int
}

func (i *Input) layout(width, height int) inputLayout {
	if height <= 0 {
		height = i.MeasureHeight(width, 1<<14)
	}
	p := inputLayout{height: height, header: -1, footer: -1}
	if width <= 0 {
		return p
	}
	if i.SearchActive() {
		if i.search.frameFits(width, height) {
			p.rules = boxRules(width, i.search.PreferredHeight())
		}
		return p
	}
	if i.PickerActive() {
		// Preserve the focused picker before spending rows on command chrome.
		// Inline completion still needs its editable command field; a modal
		// picker can occupy the entire surface on a one-row terminal.
		fieldHeight := min(3, max(0, height-2))
		if i.PickerInline() {
			fieldHeight = max(1, fieldHeight)
		}
		p.pickerHeight = min(i.picker.PreferredHeight(), max(0, height-fieldHeight))
		if i.picker.frameFits(width, p.pickerHeight) {
			p.rules = boxRules(width, p.pickerHeight)
		}
	}
	top, bottom := p.pickerHeight, height
	fieldHeight := bottom - top
	if i.composer != nil {
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
	if i.composer != nil && !i.SearchActive() {
		header, footer := i.composeLabels(strings.Count(i.composer.Value(), "\n") + 1)
		for n := range plan.rules {
			rule := &plan.rules[n]
			if rule.Vertical {
				continue
			}
			switch rule.At {
			case plan.header:
				rule.Label = ansi.Truncate(" "+header+" ", width, "")
				rule.LabelAt = width - ansi.StringWidth(rule.Label)
				rule.LabelStyle = i.styles.Warning
			case plan.footer:
				rule.Label = ansi.Truncate(" "+footer+" ", width, "")
				rule.LabelStyle = i.styles.Muted
				if i.discardPending {
					rule.LabelStyle = i.styles.Warning
				}
			}
		}
	}
	return plan.rules
}
