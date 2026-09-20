package widget

import "image"

// inputLayout describes the picker and command field separately. Both rendering
// and shared-boundary planning use these same positions, including labels.
type inputLayout struct {
	height         int
	pickerHeight   int
	results        image.Rectangle
	help           int
	body           image.Rectangle
	rules          []Rule
	header, footer int // draft label rows; -1 when unavailable
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
		// One input field stays at the bottom. Suggestions/results occupy the
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
	if i.draftEditor != nil && !i.SearchActive() && (!i.PickerActive() || i.PickerInline()) {
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

// Rules supplies line geometry for measurement, without formatting labels.
func (i *Input) Rules(width, height int) []Rule {
	if width <= 0 || height <= 0 {
		return nil
	}
	return i.layout(width, height).rules
}
