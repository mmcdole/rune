package ui

import (
	"strings"
)

// SlashPicker is the overlay for selecting slash commands.
type SlashPicker struct {
	commands []CommandInfo   // All commands
	filtered []CommandInfo   // Filtered results
	matches  []Match         // Fuzzy match data for highlighting
	query    string
	selected int
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

// SetCommands sets the commands to filter.
func (p *SlashPicker) SetCommands(commands []CommandInfo) {
	p.commands = commands
	p.filtered = commands
	p.matches = nil
	p.selected = 0
	p.scrollOff = 0
}

// SetWidth updates the picker width.
func (p *SlashPicker) SetWidth(w int) {
	p.width = w
}

// Filter updates the filtered list based on query using fuzzy matching.
func (p *SlashPicker) Filter(query string) {
	p.query = query

	if query == "" {
		p.filtered = p.commands
		p.matches = nil
		p.selected = 0
		p.scrollOff = 0
		p.adjustScroll()
		return
	}

	// Build searchable strings: "name description"
	searchStrings := make([]string, len(p.commands))
	for i, cmd := range p.commands {
		searchStrings[i] = cmd.Name + " " + cmd.Description
	}

	// Use fuzzy filter
	matches := FuzzyFilter(query, searchStrings)

	p.filtered = make([]CommandInfo, len(matches))
	p.matches = matches
	for i, m := range matches {
		p.filtered[i] = p.commands[m.Index]
	}

	p.selected = 0
	p.scrollOff = 0
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

// SelectedCommand returns the name of the currently selected command.
func (p *SlashPicker) SelectedCommand() string {
	if len(p.filtered) == 0 || p.selected < 0 || p.selected >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.selected].Name
}

// Reset clears the picker state.
func (p *SlashPicker) Reset() {
	p.query = ""
	p.filtered = p.commands
	p.matches = nil
	p.selected = 0
	p.scrollOff = 0
}

// View renders the slash picker overlay.
func (p *SlashPicker) View() string {
	if len(p.filtered) == 0 {
		return p.styles.OverlayBorder.Render("  No matching commands")
	}

	var lines []string
	start := p.scrollOff
	end := start + p.maxVisible
	if end > len(p.filtered) {
		end = len(p.filtered)
	}

	for i := start; i < end; i++ {
		cmd := p.filtered[i]
		prefix := "  "
		if i == p.selected {
			prefix = "> "
		}

		// Get match positions if available
		var positions []int
		if i < len(p.matches) {
			positions = p.matches[i].Positions
		}

		// Highlight matched characters in name and description
		// Search string was "name description" so positions map to that
		name := p.highlightText(cmd.Name, positions, 0)
		desc := ""
		if cmd.Description != "" {
			// Description starts at len(name)+1 (after the space)
			desc = " - " + p.highlightText(cmd.Description, positions, len(cmd.Name)+1)
		}

		line := "/" + name + desc

		if i == p.selected {
			lines = append(lines, p.styles.OverlaySelected.Render(prefix)+line)
		} else {
			lines = append(lines, p.styles.OverlayNormal.Render(prefix)+line)
		}
	}

	content := strings.Join(lines, "\n")
	return p.styles.OverlayBorder.Width(p.width - 4).Render(content)
}

// highlightText highlights matched positions in text, with offset adjustment.
// Positions are rune indices into the original search string ("name description").
func (p *SlashPicker) highlightText(text string, positions []int, offset int) string {
	if len(positions) == 0 {
		return text
	}

	textRunes := []rune(text)

	// Build set of positions relative to this text segment
	posSet := make(map[int]bool)
	for _, pos := range positions {
		relPos := pos - offset
		if relPos >= 0 && relPos < len(textRunes) {
			posSet[relPos] = true
		}
	}

	if len(posSet) == 0 {
		return text
	}

	var result strings.Builder
	for i, r := range textRunes {
		if posSet[i] {
			result.WriteString(p.styles.OverlayMatch.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// AliasPicker is the overlay for selecting aliases.
type AliasPicker struct {
	aliases    []AliasInfo   // All aliases
	filtered   []AliasInfo   // Filtered results
	matches    []Match       // Fuzzy match data for highlighting
	query      string
	selected   int
	maxVisible int
	scrollOff  int
	styles     Styles
	width      int
}

// NewAliasPicker creates a new alias picker.
func NewAliasPicker(styles Styles) AliasPicker {
	return AliasPicker{
		maxVisible: 10,
		styles:     styles,
	}
}

// SetAliases sets the aliases to filter.
func (p *AliasPicker) SetAliases(aliases []AliasInfo) {
	p.aliases = aliases
	p.filtered = aliases
	p.matches = nil
	p.selected = 0
	p.scrollOff = 0
}

// SetWidth updates the picker width.
func (p *AliasPicker) SetWidth(w int) {
	p.width = w
}

// Search updates the query and re-filters results.
func (p *AliasPicker) Search(query string) {
	p.query = query

	if query == "" {
		p.filtered = p.aliases
		p.matches = nil
		p.selected = 0
		p.scrollOff = 0
		p.adjustScroll()
		return
	}

	// Build searchable strings: name only (not value)
	searchStrings := make([]string, len(p.aliases))
	for i, alias := range p.aliases {
		searchStrings[i] = alias.Name
	}

	// Use fuzzy filter
	matches := FuzzyFilter(query, searchStrings)

	p.filtered = make([]AliasInfo, len(matches))
	p.matches = matches
	for i, m := range matches {
		p.filtered[i] = p.aliases[m.Index]
	}

	if p.selected >= len(p.filtered) {
		p.selected = max(0, len(p.filtered)-1)
	}
	p.adjustScroll()
}

// SelectUp moves selection up.
func (p *AliasPicker) SelectUp() {
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
func (p *AliasPicker) SelectDown() {
	if len(p.filtered) == 0 {
		return
	}
	p.selected++
	if p.selected >= len(p.filtered) {
		p.selected = 0
	}
	p.adjustScroll()
}

func (p *AliasPicker) adjustScroll() {
	if p.selected < p.scrollOff {
		p.scrollOff = p.selected
	} else if p.selected >= p.scrollOff+p.maxVisible {
		p.scrollOff = p.selected - p.maxVisible + 1
	}
}

// SelectedAlias returns the name of the currently selected alias.
func (p *AliasPicker) SelectedAlias() string {
	if len(p.filtered) == 0 || p.selected < 0 || p.selected >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.selected].Name
}

// Reset clears the picker state.
func (p *AliasPicker) Reset() {
	p.query = ""
	p.filtered = p.aliases
	p.matches = nil
	p.selected = 0
	p.scrollOff = 0
}

// View renders the alias picker overlay.
func (p *AliasPicker) View() string {
	header := p.styles.Muted.Render("Alias: ") + p.query + "█"

	if len(p.filtered) == 0 {
		content := header + "\n" + p.styles.Muted.Render("  No matches")
		return p.styles.OverlayBorder.Width(p.width - 4).Render(content)
	}

	var lines []string
	lines = append(lines, header)

	start := p.scrollOff
	end := start + p.maxVisible
	if end > len(p.filtered) {
		end = len(p.filtered)
	}

	// Available width: total - border(2) - padding(2) - prefix(2) - generous safety margin
	maxLineWidth := p.width - 12
	if maxLineWidth < 20 {
		maxLineWidth = 20
	}

	for i := start; i < end; i++ {
		alias := p.filtered[i]
		prefix := "  "
		if i == p.selected {
			prefix = "> "
		}

		// Highlight matched characters in name
		var name string
		if i < len(p.matches) && len(p.matches[i].Positions) > 0 {
			name = p.highlightName(alias.Name, p.matches[i].Positions)
		} else {
			name = alias.Name
		}

		// Format: name → value, truncated to fit
		// Arrow " → " is 3 visual chars (space + arrow + space)
		nameLen := len([]rune(alias.Name))
		arrowLen := 3
		availableForValue := maxLineWidth - nameLen - arrowLen

		value := alias.Value
		if availableForValue > 3 {
			valueRunes := []rune(value)
			if len(valueRunes) > availableForValue {
				value = string(valueRunes[:availableForValue-1]) + "…"
			}
		} else {
			value = "…"
		}

		line := name + " → " + value

		if i == p.selected {
			lines = append(lines, p.styles.OverlaySelected.Render(prefix)+line)
		} else {
			lines = append(lines, p.styles.OverlayNormal.Render(prefix)+line)
		}
	}

	content := strings.Join(lines, "\n")
	return p.styles.OverlayBorder.Width(p.width - 4).Render(content)
}

// highlightName highlights matched positions in the alias name.
// Positions are rune indices from the fuzzy matcher.
func (p *AliasPicker) highlightName(name string, positions []int) string {
	if len(positions) == 0 {
		return name
	}

	posSet := make(map[int]bool)
	for _, pos := range positions {
		posSet[pos] = true
	}

	nameRunes := []rune(name)
	var result strings.Builder
	for i, r := range nameRunes {
		if posSet[i] {
			result.WriteString(p.styles.OverlayMatch.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// HistoryMatch represents a fuzzy match result.
type HistoryMatch struct {
	Index     int
	Command   string
	Score     int
	Positions []int // Matched character positions
}

// HistoryPicker is the overlay for fuzzy history search.
type HistoryPicker struct {
	history    []string
	query      string
	matches    []HistoryMatch
	selected   int
	maxVisible int
	scrollOff  int
	styles     Styles
	width      int
}

// NewHistoryPicker creates a new history picker overlay.
func NewHistoryPicker(styles Styles) HistoryPicker {
	return HistoryPicker{
		maxVisible: 10,
		styles:     styles,
	}
}

// SetWidth updates the picker width.
func (h *HistoryPicker) SetWidth(w int) {
	h.width = w
}

// SetHistory provides the command history to search (deduplicated).
func (h *HistoryPicker) SetHistory(history []string) {
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

	h.history = deduped
}

// Search updates the query and re-filters results.
func (h *HistoryPicker) Search(query string) {
	h.query = query
	h.matches = make([]HistoryMatch, 0)

	if query == "" {
		// Show recent history (most recent first)
		for i := len(h.history) - 1; i >= 0 && len(h.matches) < 50; i-- {
			h.matches = append(h.matches, HistoryMatch{
				Index:   i,
				Command: h.history[i],
				Score:   0,
			})
		}
	} else {
		// Use FuzzyFilter for consistent matching
		fuzzyMatches := FuzzyFilter(query, h.history)

		for _, m := range fuzzyMatches {
			if len(h.matches) >= 50 {
				break
			}
			h.matches = append(h.matches, HistoryMatch{
				Index:     m.Index,
				Command:   m.Text,
				Score:     m.Score,
				Positions: m.Positions,
			})
		}
	}

	if h.selected >= len(h.matches) {
		h.selected = max(0, len(h.matches)-1)
	}
	h.adjustScroll()
}

// SelectUp moves selection up.
func (h *HistoryPicker) SelectUp() {
	if len(h.matches) == 0 {
		return
	}
	h.selected--
	if h.selected < 0 {
		h.selected = len(h.matches) - 1
	}
	h.adjustScroll()
}

// SelectDown moves selection down.
func (h *HistoryPicker) SelectDown() {
	if len(h.matches) == 0 {
		return
	}
	h.selected++
	if h.selected >= len(h.matches) {
		h.selected = 0
	}
	h.adjustScroll()
}

func (h *HistoryPicker) adjustScroll() {
	if h.selected < h.scrollOff {
		h.scrollOff = h.selected
	} else if h.selected >= h.scrollOff+h.maxVisible {
		h.scrollOff = h.selected - h.maxVisible + 1
	}
}

// Selected returns the currently selected match, or nil if none.
func (h *HistoryPicker) Selected() *HistoryMatch {
	if len(h.matches) == 0 || h.selected < 0 || h.selected >= len(h.matches) {
		return nil
	}
	return &h.matches[h.selected]
}

// Reset clears the picker state.
func (h *HistoryPicker) Reset() {
	h.query = ""
	h.matches = nil
	h.selected = 0
	h.scrollOff = 0
}

// View renders the history picker overlay.
func (h *HistoryPicker) View() string {
	header := h.styles.Muted.Render("History: ") + h.query + "█"

	if len(h.matches) == 0 {
		content := header + "\n" + h.styles.Muted.Render("  No matches")
		return h.styles.OverlayBorder.Width(h.width - 4).Render(content)
	}

	var lines []string
	lines = append(lines, header)

	visible := h.matches
	if len(visible) > h.maxVisible {
		end := h.scrollOff + h.maxVisible
		if end > len(visible) {
			end = len(visible)
		}
		visible = visible[h.scrollOff:end]
	}

	for i, match := range visible {
		actualIdx := h.scrollOff + i
		prefix := "  "
		if actualIdx == h.selected {
			prefix = "> "
		}

		// Highlight matched characters
		line := h.highlightMatch(match)

		if actualIdx == h.selected {
			lines = append(lines, h.styles.OverlaySelected.Render(prefix+line))
		} else {
			lines = append(lines, h.styles.OverlayNormal.Render(prefix)+line)
		}
	}

	content := strings.Join(lines, "\n")
	return h.styles.OverlayBorder.Width(h.width - 4).Render(content)
}

func (h *HistoryPicker) highlightMatch(match HistoryMatch) string {
	if len(match.Positions) == 0 {
		return match.Command
	}

	// Create a set of matched positions (rune indices)
	posSet := make(map[int]bool)
	for _, p := range match.Positions {
		posSet[p] = true
	}

	cmdRunes := []rune(match.Command)
	var result strings.Builder
	for i, r := range cmdRunes {
		if posSet[i] {
			result.WriteString(h.styles.OverlayMatch.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

