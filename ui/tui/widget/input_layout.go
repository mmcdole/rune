package widget

import "image"

// InputLayoutState is a comparable description of input's geometry dependencies.
// Cursor, selection, labels, and single-line text do not change geometry.
// Draft edits invalidate all measured widths, not just the current allocation.
type InputLayoutState struct {
	overlay  inputOverlay
	draft    *draftEditor
	revision uint64
	results  int
}

func (i *Input) LayoutState() InputLayoutState {
	state := InputLayoutState{overlay: i.overlay}
	if i.SearchActive() {
		state.results = i.search.resultHeight()
		return state
	}
	if i.PickerActive() {
		state.results = i.picker.resultHeight()
		if !i.PickerInline() {
			return state
		}
	}
	state.draft = i.draftEditor
	if state.draft != nil {
		state.revision = state.draft.revision
	}
	return state
}

// inputLayout contains only content geometry. The surrounding layout owns
// outside borders; an overlay has one internal separator above its query.
type inputLayout struct {
	results   image.Rectangle
	help      int
	body      image.Rectangle
	separator int // -1 when there is no room for a separator
}

func (i *Input) layout(width, height int) inputLayout {
	p := inputLayout{help: -1, separator: -1}
	if width <= 0 || height <= 0 {
		return p
	}
	top := 0
	if i.PickerActive() || i.SearchActive() {
		top = height - 1 // preserve one editable row below the results
		end := top
		if height >= 3 {
			end--
			p.separator = end
		}
		p.results = image.Rect(0, 0, width, end)
		if i.SearchActive() && p.results.Dy() > 1 {
			p.results.Max.Y--
			p.help = p.results.Max.Y
		}
	}
	p.body = image.Rect(0, top, width, height)
	return p
}

// SeparatorRow is the internal rule between results and editable content.
// Outside borders are not part of the widget's allocated content rectangle.
func (i *Input) SeparatorRow(width, height int) int {
	return i.layout(width, height).separator
}
