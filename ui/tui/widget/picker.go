package widget

import (
	"strings"

	"github.com/drake/rune/ui/tui/style"
	"github.com/drake/rune/ui/tui/util"
)

// PickerItem represents a row in the picker.
type PickerItem interface {
	FilterValue() string
	GetText() string
	GetDescription() string
	GetValue() string
	MatchesDescription() bool
}

// PickerConfig holds picker configuration.
type PickerConfig struct {
	MaxVisible int
	Header     string
	EmptyText  string
}

// Picker is a generic fuzzy-filtering selector.
type Picker[T PickerItem] struct {
	items     []T
	filtered  []T
	matches   []util.Match
	query     string
	selected  int
	scrollOff int
	config    PickerConfig
	styles    style.Styles
	width     int
}

// NewPicker creates a new picker.
func NewPicker[T PickerItem](config PickerConfig, styles style.Styles) *Picker[T] {
	if config.MaxVisible == 0 {
		config.MaxVisible = 10
	}
	if config.EmptyText == "" {
		config.EmptyText = "No matches"
	}
	return &Picker[T]{
		config: config,
		styles: styles,
	}
}

// SetItems sets the items to filter.
func (p *Picker[T]) SetItems(items []T) {
	p.items = items
	p.Reset()
}

// SetWidth updates the picker width.
func (p *Picker[T]) SetWidth(w int) {
	p.width = w
}

// SetHeader updates the header text.
func (p *Picker[T]) SetHeader(header string) {
	p.config.Header = header
}

// Query returns the current filter query.
func (p *Picker[T]) Query() string {
	return p.query
}

// Filter updates the filtered list based on query.
func (p *Picker[T]) Filter(query string) {
	p.query = query

	if query == "" {
		p.filtered = p.items
		p.matches = nil
		p.selected = 0
		p.scrollOff = 0
		p.adjustScroll()
		return
	}

	searchStrings := make([]string, len(p.items))
	for i, item := range p.items {
		searchStrings[i] = item.FilterValue()
	}

	rawMatches := util.FuzzyFilter(query, searchStrings)

	p.filtered = make([]T, len(rawMatches))
	p.matches = rawMatches
	for i, match := range rawMatches {
		p.filtered[i] = p.items[match.Index]
	}

	if p.selected >= len(p.filtered) {
		p.selected = max(0, len(p.filtered)-1)
	}
	p.scrollOff = 0
	p.adjustScroll()
}

// SelectUp moves selection up with wraparound.
func (p *Picker[T]) SelectUp() {
	if len(p.filtered) == 0 {
		return
	}
	p.selected--
	if p.selected < 0 {
		p.selected = len(p.filtered) - 1
	}
	p.adjustScroll()
}

// SelectDown moves selection down with wraparound.
func (p *Picker[T]) SelectDown() {
	if len(p.filtered) == 0 {
		return
	}
	p.selected++
	if p.selected >= len(p.filtered) {
		p.selected = 0
	}
	p.adjustScroll()
}

func (p *Picker[T]) adjustScroll() {
	if p.selected < p.scrollOff {
		p.scrollOff = p.selected
	} else if p.selected >= p.scrollOff+p.config.MaxVisible {
		p.scrollOff = p.selected - p.config.MaxVisible + 1
	}
}

// Reset clears the picker state.
func (p *Picker[T]) Reset() {
	p.query = ""
	p.filtered = p.items
	p.matches = nil
	p.selected = 0
	p.scrollOff = 0
}

// Selected returns the currently selected item.
func (p *Picker[T]) Selected() (T, bool) {
	var zero T
	if len(p.filtered) == 0 || p.selected < 0 || p.selected >= len(p.filtered) {
		return zero, false
	}
	return p.filtered[p.selected], true
}

// Height returns the rendered height including border.
func (p *Picker[T]) Height() int {
	h := len(p.filtered)
	if h > p.config.MaxVisible {
		h = p.config.MaxVisible
	}
	if h == 0 {
		h = 1 // "No matches" placeholder
	}

	if p.config.Header != "" {
		h++
	}

	h += 2 // border
	return h
}

// View renders the picker overlay.
func (p *Picker[T]) View() string {
	var lines []string

	if p.config.Header != "" {
		header := p.styles.Muted.Render(p.config.Header) + p.query + "█"
		lines = append(lines, header)
	}

	if len(p.filtered) == 0 {
		lines = append(lines, p.styles.Muted.Render("  "+p.config.EmptyText))
		content := strings.Join(lines, "\n")
		return p.styles.OverlayBorder.Width(p.width - 4).Render(content)
	}

	start := p.scrollOff
	end := start + p.config.MaxVisible
	if end > len(p.filtered) {
		end = len(p.filtered)
	}

	for i := start; i < end; i++ {
		item := p.filtered[i]
		selected := i == p.selected

		var positions []int
		if i < len(p.matches) {
			positions = p.matches[i].Positions
		}

		line := p.renderItem(item, p.width-4, selected, positions)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return p.styles.OverlayBorder.Width(p.width - 4).Render(content)
}

func (p *Picker[T]) renderItem(item T, width int, selected bool, matches []int) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	matchSet := make(map[int]bool, len(matches))
	for _, pos := range matches {
		matchSet[pos] = true
	}

	var result strings.Builder

	text := item.GetText()
	desc := item.GetDescription()
	matchDesc := item.MatchesDescription()

	for idx, r := range text {
		ch := string(r)
		if matchSet[idx] && selected {
			result.WriteString(p.styles.OverlayMatchSelected.Render(ch))
		} else if matchSet[idx] {
			result.WriteString(p.styles.OverlayMatch.Render(ch))
		} else if selected {
			result.WriteString(p.styles.OverlaySelected.Render(ch))
		} else {
			result.WriteString(p.styles.OverlayNormal.Render(ch))
		}
	}

	if desc != "" {
		sep := " - "
		if selected {
			result.WriteString(p.styles.OverlaySelected.Render(sep))
		} else {
			result.WriteString(p.styles.OverlayNormal.Render(sep))
		}

		descOffset := len(text) + 1
		for idx, r := range desc {
			ch := string(r)
			isMatch := matchDesc && matchSet[descOffset+idx]
			if isMatch && selected {
				result.WriteString(p.styles.OverlayMatchSelected.Render(ch))
			} else if isMatch {
				result.WriteString(p.styles.OverlayMatch.Render(ch))
			} else if selected {
				result.WriteString(p.styles.OverlaySelected.Render(ch))
			} else {
				result.WriteString(p.styles.OverlayNormal.Render(ch))
			}
		}
	}

	var prefixStyled string
	if selected {
		prefixStyled = p.styles.OverlaySelected.Render(prefix)
	} else {
		prefixStyled = p.styles.OverlayNormal.Render(prefix)
	}

	return prefixStyled + result.String()
}
