package widget

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// PickerConfig holds picker configuration.
type PickerConfig struct {
	MaxVisible int
	Header     string
	EmptyText  string
}

// Picker is a fuzzy-filtering selector for PickerItems.
type Picker struct {
	items        []ui.PickerItem
	filtered     []ui.PickerItem
	matches      []util.Match
	query        string
	selected     int
	scrollOff    int
	config       PickerConfig
	styles       style.Styles
	width        int
	queryVisible bool
}

// NewPicker creates a new picker.
func NewPicker(config PickerConfig, styles style.Styles) *Picker {
	if config.MaxVisible == 0 {
		config.MaxVisible = 10
	}
	if config.EmptyText == "" {
		config.EmptyText = "No matches"
	}
	return &Picker{
		config:       config,
		queryVisible: config.Header != "",
		styles:       styles,
	}
}

// SetItems sets the items to filter.
func (p *Picker) SetItems(items []ui.PickerItem) {
	p.items = items
	p.Reset()
}

// SetWidth updates the picker width.
func (p *Picker) SetWidth(w int) {
	p.width = w
}

// SetHeader updates the header text.
func (p *Picker) SetHeader(header string) {
	p.config.Header = header
	p.queryVisible = header != ""
}

// Query returns the current filter query.
func (p *Picker) Query() string {
	return p.query
}

// Filter updates the filtered list based on query.
func (p *Picker) Filter(query string) {
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

	p.filtered = make([]ui.PickerItem, len(rawMatches))
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
func (p *Picker) SelectUp() {
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
func (p *Picker) SelectDown() {
	if len(p.filtered) == 0 {
		return
	}
	p.selected++
	if p.selected >= len(p.filtered) {
		p.selected = 0
	}
	p.adjustScroll()
}

func (p *Picker) adjustScroll() {
	if p.selected < p.scrollOff {
		p.scrollOff = p.selected
	} else if p.selected >= p.scrollOff+p.config.MaxVisible {
		p.scrollOff = p.selected - p.config.MaxVisible + 1
	}
}

// Reset clears the picker state.
func (p *Picker) Reset() {
	p.query = ""
	p.filtered = p.items
	p.matches = nil
	p.selected = 0
	p.scrollOff = 0
}

// Selected returns the currently selected item.
func (p *Picker) Selected() (ui.PickerItem, bool) {
	if len(p.filtered) == 0 || p.selected < 0 || p.selected >= len(p.filtered) {
		return ui.PickerItem{}, false
	}
	return p.filtered[p.selected], true
}

// PreferredHeight returns the rendered height including border.
func (p *Picker) PreferredHeight() int {
	h := len(p.filtered)
	if h > p.config.MaxVisible {
		h = p.config.MaxVisible
	}
	if h == 0 {
		h = 1 // "No matches" placeholder
	}

	if p.queryVisible {
		h++
	}

	h += 2 // border
	return h
}

// View renders the full picker at its preferred height.
func (p *Picker) View() string { return p.view(p.PreferredHeight(), true) }

func (p *Picker) contentView(height int) string { return p.view(height, false) }

func (p *Picker) frameFits(width, height int) bool {
	chrome := p.styles.OverlayBorder.GetHorizontalBorderSize() + p.styles.OverlayBorder.GetHorizontalPadding()
	minimum := 3 // border + one result
	if p.queryVisible {
		minimum++
	}
	return width > chrome && height >= minimum
}

// view reduces the result window around the selection before dropping its
// frame. Even a one-row modal picker retains its editable query and cursor.
func (p *Picker) view(height int, paintFrame bool) string {
	if height <= 0 {
		return ""
	}
	width := max(1, p.width)
	framed := p.frameFits(width, height)
	contentWidth, contentHeight := width, height
	if framed {
		contentWidth -= p.styles.OverlayBorder.GetHorizontalBorderSize() + p.styles.OverlayBorder.GetHorizontalPadding()
		contentHeight -= 2
	}
	lines := make([]string, 0, contentHeight)
	if p.queryVisible {
		lines = append(lines, p.queryLine(contentWidth))
	}
	limit := max(0, contentHeight-len(lines))
	if limit > 0 {
		if len(p.filtered) == 0 {
			lines = append(lines, clipRow(p.styles.Muted.Render("  "+text.VisualizeTerminalControls(p.config.EmptyText, false)), contentWidth))
		} else {
			visible := min(limit, len(p.filtered))
			start := min(p.scrollOff, p.selected)
			start = max(start, p.selected-visible+1)
			start = max(0, min(start, len(p.filtered)-visible))
			for i := start; i < start+visible; i++ {
				var positions []int
				if i < len(p.matches) {
					positions = p.matches[i].Positions
				}
				lines = append(lines, p.renderItem(p.filtered[i], contentWidth, i == p.selected, positions))
			}
		}
	}
	for len(lines) < contentHeight {
		lines = append(lines, "")
	}
	content := strings.Join(lines, "\n")
	if framed {
		if !paintFrame {
			padding := 1 + p.styles.OverlayBorder.GetPaddingLeft()
			for n := range lines {
				lines[n] = strings.Repeat(" ", padding) + lines[n]
			}
			return "\n" + strings.Join(lines, "\n") + "\n"
		}
		return p.styles.OverlayBorder.Width(width).Render(content)
	}
	return content
}

func (p *Picker) queryLine(width int) string {
	header := text.VisualizeTerminalControls(p.config.Header, false)
	if ansi.StringWidth(header)+1 >= width {
		header = ""
	}
	query := text.VisualizeTerminalControls(p.query, false)
	available := max(0, width-ansi.StringWidth(header)-1)
	query = ansi.TruncateLeft(query, max(0, ansi.StringWidth(query)-available), "")
	return p.styles.Muted.Render(header) + query + "█"
}

func (p *Picker) renderItem(item ui.PickerItem, width int, selected bool, matches []int) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	// Match positions from the fuzzy scorer are rune indices into
	// FilterValue() (text, or "text description"); index by rune, not
	// byte, or highlights misalign on multi-byte input.
	matchSet := make(map[int]bool, len(matches))
	for _, pos := range matches {
		matchSet[pos] = true
	}

	var result strings.Builder

	itemText := text.VisualizeTerminalControls(item.GetText(), false)
	desc := text.VisualizeTerminalControls(item.GetDescription(), false)
	matchDesc := item.MatchesDescription()
	textRunes := []rune(itemText)

	for idx, r := range textRunes {
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

		descOffset := len(textRunes) + 1
		for idx, r := range []rune(desc) {
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

	if width < 1 {
		width = 1
	}
	return clipRow(prefixStyled+result.String(), width)
}
