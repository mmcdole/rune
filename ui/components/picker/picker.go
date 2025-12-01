package picker

import (
	"strings"

	"github.com/drake/rune/ui/style"
	"github.com/drake/rune/ui/util"
)

// Config holds picker configuration.
type Config struct {
	MaxVisible int    // Maximum number of visible items
	Header     string // Optional header text (e.g., "History: ")
	EmptyText  string // Text to show when no matches (default: "No matches")
}

// Model is a generic fuzzy-filtering selector.
type Model[T Item] struct {
	items      []T
	filtered   []T
	matches    []util.Match
	query      string
	selected   int
	scrollOff  int
	config     Config
	styles     style.Styles
	width      int
}

// New creates a new picker with the given configuration.
func New[T Item](config Config, styles style.Styles) *Model[T] {
	if config.MaxVisible == 0 {
		config.MaxVisible = 10
	}
	if config.EmptyText == "" {
		config.EmptyText = "No matches"
	}
	return &Model[T]{
		config: config,
		styles: styles,
	}
}

// SetItems sets the items to filter.
func (m *Model[T]) SetItems(items []T) {
	m.items = items
	m.Reset()
}

// SetWidth updates the picker width.
func (m *Model[T]) SetWidth(w int) {
	m.width = w
}

// Width returns the current width.
func (m *Model[T]) Width() int {
	return m.width
}

// Query returns the current filter query.
func (m *Model[T]) Query() string {
	return m.query
}

// Filter updates the filtered list based on query.
func (m *Model[T]) Filter(query string) {
	m.query = query

	if query == "" {
		m.filtered = m.items
		m.matches = nil
		m.selected = 0
		m.scrollOff = 0
		m.adjustScroll()
		return
	}

	// Build searchable strings from items
	searchStrings := make([]string, len(m.items))
	for i, item := range m.items {
		searchStrings[i] = item.FilterValue()
	}

	// Apply fuzzy filter
	rawMatches := util.FuzzyFilter(query, searchStrings)

	m.filtered = make([]T, len(rawMatches))
	m.matches = rawMatches
	for i, match := range rawMatches {
		m.filtered[i] = m.items[match.Index]
	}

	// Reset selection if out of bounds
	if m.selected >= len(m.filtered) {
		m.selected = max(0, len(m.filtered)-1)
	}
	m.scrollOff = 0
	m.adjustScroll()
}

// SelectUp moves selection up with wraparound.
func (m *Model[T]) SelectUp() {
	if len(m.filtered) == 0 {
		return
	}
	m.selected--
	if m.selected < 0 {
		m.selected = len(m.filtered) - 1
	}
	m.adjustScroll()
}

// SelectDown moves selection down with wraparound.
func (m *Model[T]) SelectDown() {
	if len(m.filtered) == 0 {
		return
	}
	m.selected++
	if m.selected >= len(m.filtered) {
		m.selected = 0
	}
	m.adjustScroll()
}

func (m *Model[T]) adjustScroll() {
	if m.selected < m.scrollOff {
		m.scrollOff = m.selected
	} else if m.selected >= m.scrollOff+m.config.MaxVisible {
		m.scrollOff = m.selected - m.config.MaxVisible + 1
	}
}

// Reset clears the picker state.
func (m *Model[T]) Reset() {
	m.query = ""
	m.filtered = m.items
	m.matches = nil
	m.selected = 0
	m.scrollOff = 0
}

// Selected returns the currently selected item, or false if none.
func (m *Model[T]) Selected() (T, bool) {
	var zero T
	if len(m.filtered) == 0 || m.selected < 0 || m.selected >= len(m.filtered) {
		return zero, false
	}
	return m.filtered[m.selected], true
}

// SelectedIndex returns the current selection index.
func (m *Model[T]) SelectedIndex() int {
	return m.selected
}

// Count returns the number of filtered items.
func (m *Model[T]) Count() int {
	return len(m.filtered)
}

// IsEmpty returns true if there are no filtered items.
func (m *Model[T]) IsEmpty() bool {
	return len(m.filtered) == 0
}

// Height returns the rendered height of this picker (including border).
func (m *Model[T]) Height() int {
	h := len(m.filtered)
	if h > m.config.MaxVisible {
		h = m.config.MaxVisible
	}
	if h == 0 {
		h = 1 // "No matches" placeholder
	}

	// Add header if present
	if m.config.Header != "" {
		h++
	}

	// Add border (top + bottom)
	h += 2

	return h
}

// View renders the picker overlay.
func (m *Model[T]) View() string {
	var lines []string

	// Add header if present
	if m.config.Header != "" {
		header := m.styles.Muted.Render(m.config.Header) + m.query + "█"
		lines = append(lines, header)
	}

	// Handle empty state
	if len(m.filtered) == 0 {
		lines = append(lines, m.styles.Muted.Render("  "+m.config.EmptyText))
		content := strings.Join(lines, "\n")
		return m.styles.OverlayBorder.Width(m.width - 4).Render(content)
	}

	// Calculate visible range
	start := m.scrollOff
	end := start + m.config.MaxVisible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	// Render visible items
	for i := start; i < end; i++ {
		item := m.filtered[i]
		selected := i == m.selected

		// Get match positions if available
		var positions []int
		if i < len(m.matches) {
			positions = m.matches[i].Positions
		}

		line := item.Render(m.width-4, selected, positions, m.styles)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return m.styles.OverlayBorder.Width(m.width - 4).Render(content)
}
