package widget

import "image"

// inputLayout describes the picker and command field separately. Both rendering
// and shared-boundary planning use these same positions, including labels.
type inputLayout struct {
	results        image.Rectangle
	help           int
	body           image.Rectangle
	ruleRows       []int
	header, footer int // draft label rows; -1 when unavailable
}

func (i *Input) layout(width, height int) inputLayout {
	p := inputLayout{help: -1, header: -1, footer: -1}
	if width <= 0 || height <= 0 {
		return p
	}
	pickerHeight := 0
	if i.PickerActive() || i.SearchActive() {
		// One input field stays at the bottom. Suggestions/results occupy the
		// rows above it; Input owns their shared separators, with no box.
		fieldHeight := min(3, max(1, height-1))
		pickerHeight = max(0, height-fieldHeight)
		start := 0
		if pickerHeight > 1 {
			p.ruleRows = append(p.ruleRows, 0)
			start = 1
		}
		p.results = image.Rect(0, start, width, pickerHeight)
		if i.SearchActive() && p.results.Dy() > 1 {
			p.results.Max.Y--
			p.help = p.results.Max.Y
		}
	}
	top, bottom := pickerHeight, height
	fieldHeight := bottom - top
	if i.draftEditor != nil && !i.SearchActive() && (!i.PickerActive() || i.PickerInline()) {
		if fieldHeight >= 2 {
			p.header = top
			p.ruleRows = append(p.ruleRows, top)
			top++
		}
		if fieldHeight >= 3 {
			bottom--
			p.footer = bottom
			p.ruleRows = append(p.ruleRows, bottom)
		}
	} else {
		if fieldHeight >= 3 {
			p.ruleRows = append(p.ruleRows, top)
			top++
		}
		if fieldHeight >= 2 {
			bottom--
			p.ruleRows = append(p.ruleRows, bottom)
		}
	}
	p.body = image.Rect(0, top, width, bottom)
	return p
}

// RuleRows returns local rows occupied by full-width horizontal lines.
// Measurement and rendering share these positions without formatting labels.
func (i *Input) RuleRows(width, height int) []int {
	return i.layout(width, height).ruleRows
}
