package ui

import (
	"sort"
	"strings"
	"unicode"
)

// SlashPicker is the overlay for selecting slash commands.
// It filters lines from the "commands" pane.
type SlashPicker struct {
	lines      []string // All command lines from pane
	filtered   []string // Filtered results
	query      string
	selected   int
	maxVisible int
	scrollOff  int
	styles     Styles
	width      int
}

// NewSlashPicker creates a new slash command picker.
func NewSlashPicker(styles Styles) SlashPicker {
	return SlashPicker{
		maxVisible: 8,
		styles:     styles,
	}
}

// SetLines sets the command lines to filter (from pane content).
func (p *SlashPicker) SetLines(lines []string) {
	p.lines = lines
	p.filtered = lines
	p.selected = 0
	p.scrollOff = 0
}

// SetWidth updates the picker width.
func (p *SlashPicker) SetWidth(w int) {
	p.width = w
}

// Filter updates the filtered list based on query.
func (p *SlashPicker) Filter(query string) {
	p.query = query
	if query == "" {
		p.filtered = p.lines
		p.selected = 0
		p.scrollOff = 0
		return
	}

	queryLower := strings.ToLower(query)
	p.filtered = make([]string, 0)

	for _, line := range p.lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			p.filtered = append(p.filtered, line)
		}
	}

	if p.selected >= len(p.filtered) {
		p.selected = max(0, len(p.filtered)-1)
	}
	p.adjustScroll()
}

// SelectUp moves selection up.
func (p *SlashPicker) SelectUp() {
	if len(p.filtered) == 0 {
		return
	}
	p.selected--
	if p.selected < 0 {
		p.selected = len(p.filtered) - 1
	}
	p.adjustScroll()
}

// SelectDown moves selection down.
func (p *SlashPicker) SelectDown() {
	if len(p.filtered) == 0 {
		return
	}
	p.selected++
	if p.selected >= len(p.filtered) {
		p.selected = 0
	}
	p.adjustScroll()
}

func (p *SlashPicker) adjustScroll() {
	if p.selected < p.scrollOff {
		p.scrollOff = p.selected
	} else if p.selected >= p.scrollOff+p.maxVisible {
		p.scrollOff = p.selected - p.maxVisible + 1
	}
}

// Selected returns the currently selected line, or empty string if none.
func (p *SlashPicker) Selected() string {
	if len(p.filtered) == 0 || p.selected < 0 || p.selected >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.selected]
}

// SelectedCommand extracts just the command name from the selected line.
// Assumes format "/command - description" or "/command"
func (p *SlashPicker) SelectedCommand() string {
	line := p.Selected()
	if line == "" {
		return ""
	}

	// Strip leading /
	line = strings.TrimPrefix(line, "/")

	// Take just the command name (before space or dash)
	if idx := strings.IndexAny(line, " -"); idx > 0 {
		return line[:idx]
	}
	return strings.TrimSpace(line)
}

// Reset clears the picker state.
func (p *SlashPicker) Reset() {
	p.query = ""
	p.filtered = p.lines
	p.selected = 0
	p.scrollOff = 0
}

// View renders the slash picker overlay.
func (p *SlashPicker) View() string {
	if len(p.filtered) == 0 {
		return p.styles.OverlayBorder.Render("  No matching commands")
	}

	var lines []string
	visible := p.filtered
	if len(visible) > p.maxVisible {
		end := p.scrollOff + p.maxVisible
		if end > len(visible) {
			end = len(visible)
		}
		visible = visible[p.scrollOff:end]
	}

	for i, line := range visible {
		actualIdx := p.scrollOff + i
		prefix := "  "
		style := p.styles.OverlayNormal
		if actualIdx == p.selected {
			prefix = "> "
			style = p.styles.OverlaySelected
		}

		lines = append(lines, style.Render(prefix+line))
	}

	content := strings.Join(lines, "\n")
	return p.styles.OverlayBorder.Width(p.width - 4).Render(content)
}

// HistoryMatch represents a fuzzy match result.
type HistoryMatch struct {
	Index     int
	Command   string
	Score     int
	Positions []int // Matched character positions
}

// FuzzySearch is the overlay for fuzzy history search.
type FuzzySearch struct {
	history    []string
	query      string
	matches    []HistoryMatch
	selected   int
	maxVisible int
	scrollOff  int
	styles     Styles
	width      int
}

// NewFuzzySearch creates a new fuzzy search overlay.
func NewFuzzySearch(styles Styles) FuzzySearch {
	return FuzzySearch{
		maxVisible: 10,
		styles:     styles,
	}
}

// SetWidth updates the search width.
func (f *FuzzySearch) SetWidth(w int) {
	f.width = w
}

// SetHistory provides the command history to search (deduplicated).
func (f *FuzzySearch) SetHistory(history []string) {
	// Deduplicate: keep only most recent occurrence of each command
	seen := make(map[string]bool)
	deduped := make([]string, 0, len(history))

	// Iterate backwards (most recent first)
	for i := len(history) - 1; i >= 0; i-- {
		cmd := history[i]
		if !seen[cmd] {
			seen[cmd] = true
			deduped = append(deduped, cmd)
		}
	}

	// Reverse to restore chronological order (oldest first, newest last)
	for i, j := 0, len(deduped)-1; i < j; i, j = i+1, j-1 {
		deduped[i], deduped[j] = deduped[j], deduped[i]
	}

	f.history = deduped
}

// Search updates the query and re-filters results.
func (f *FuzzySearch) Search(query string) {
	f.query = query
	f.matches = make([]HistoryMatch, 0)

	if query == "" {
		// Show recent history
		for i := len(f.history) - 1; i >= 0 && len(f.matches) < 50; i-- {
			f.matches = append(f.matches, HistoryMatch{
				Index:   i,
				Command: f.history[i],
				Score:   0,
			})
		}
	} else {
		// Fuzzy match
		for i, cmd := range f.history {
			if matched, score, positions := FuzzyMatch(query, cmd); matched {
				f.matches = append(f.matches, HistoryMatch{
					Index:     i,
					Command:   cmd,
					Score:     score,
					Positions: positions,
				})
			}
		}

		// Sort by score (higher is better), then by recency
		sort.Slice(f.matches, func(i, j int) bool {
			if f.matches[i].Score != f.matches[j].Score {
				return f.matches[i].Score > f.matches[j].Score
			}
			return f.matches[i].Index > f.matches[j].Index
		})

		// Limit results
		if len(f.matches) > 50 {
			f.matches = f.matches[:50]
		}
	}

	if f.selected >= len(f.matches) {
		f.selected = max(0, len(f.matches)-1)
	}
	f.adjustScroll()
}

// SelectUp moves selection up.
func (f *FuzzySearch) SelectUp() {
	if len(f.matches) == 0 {
		return
	}
	f.selected--
	if f.selected < 0 {
		f.selected = len(f.matches) - 1
	}
	f.adjustScroll()
}

// SelectDown moves selection down.
func (f *FuzzySearch) SelectDown() {
	if len(f.matches) == 0 {
		return
	}
	f.selected++
	if f.selected >= len(f.matches) {
		f.selected = 0
	}
	f.adjustScroll()
}

func (f *FuzzySearch) adjustScroll() {
	if f.selected < f.scrollOff {
		f.scrollOff = f.selected
	} else if f.selected >= f.scrollOff+f.maxVisible {
		f.scrollOff = f.selected - f.maxVisible + 1
	}
}

// Selected returns the currently selected match, or nil if none.
func (f *FuzzySearch) Selected() *HistoryMatch {
	if len(f.matches) == 0 || f.selected < 0 || f.selected >= len(f.matches) {
		return nil
	}
	return &f.matches[f.selected]
}

// Reset clears the search state.
func (f *FuzzySearch) Reset() {
	f.query = ""
	f.matches = nil
	f.selected = 0
	f.scrollOff = 0
}

// View renders the fuzzy search overlay.
func (f *FuzzySearch) View() string {
	header := f.styles.Muted.Render("Search: ") + f.query + "█"

	if len(f.matches) == 0 {
		content := header + "\n" + f.styles.Muted.Render("  No matches")
		return f.styles.OverlayBorder.Width(f.width - 4).Render(content)
	}

	var lines []string
	lines = append(lines, header)

	visible := f.matches
	if len(visible) > f.maxVisible {
		end := f.scrollOff + f.maxVisible
		if end > len(visible) {
			end = len(visible)
		}
		visible = visible[f.scrollOff:end]
	}

	for i, match := range visible {
		actualIdx := f.scrollOff + i
		prefix := "  "
		if actualIdx == f.selected {
			prefix = "> "
		}

		// Highlight matched characters
		line := f.highlightMatch(match)

		if actualIdx == f.selected {
			lines = append(lines, f.styles.OverlaySelected.Render(prefix+line))
		} else {
			lines = append(lines, f.styles.OverlayNormal.Render(prefix)+line)
		}
	}

	content := strings.Join(lines, "\n")
	return f.styles.OverlayBorder.Width(f.width - 4).Render(content)
}

func (f *FuzzySearch) highlightMatch(match HistoryMatch) string {
	if len(match.Positions) == 0 {
		return match.Command
	}

	// Create a set of matched positions
	posSet := make(map[int]bool)
	for _, p := range match.Positions {
		posSet[p] = true
	}

	var result strings.Builder
	for i, r := range match.Command {
		if posSet[i] {
			result.WriteString(f.styles.OverlayMatch.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// FuzzyMatch performs fuzzy matching and returns match status, score, and positions.
func FuzzyMatch(pattern, text string) (bool, int, []int) {
	if pattern == "" {
		return true, 0, nil
	}

	patternLower := strings.ToLower(pattern)
	textLower := strings.ToLower(text)

	var positions []int
	score := 0
	pIdx := 0

	for i, c := range textLower {
		if pIdx < len(patternLower) && byte(c) == patternLower[pIdx] {
			positions = append(positions, i)

			// Score bonuses
			if i == 0 {
				score += 10 // Start of string
			} else if i > 0 && (text[i-1] == ' ' || text[i-1] == '/' || text[i-1] == '_' || text[i-1] == '-') {
				score += 8 // Start of word
			} else if unicode.IsUpper(rune(text[i])) {
				score += 6 // CamelCase boundary
			} else if len(positions) > 1 && positions[len(positions)-2] == i-1 {
				score += 4 // Consecutive match
			}

			pIdx++
		}
	}

	if pIdx < len(patternLower) {
		return false, 0, nil
	}

	// Prefer shorter matches
	score -= len(text) - len(pattern)

	return true, score, positions
}
