package ui

import (
	"strings"
)

// ScrollMode indicates whether the viewport is live or scrolled back.
type ScrollMode int

const (
	// ModeLive means the viewport is pinned to the bottom, showing newest lines.
	ModeLive ScrollMode = iota
	// ModeScrolled means the user has scrolled up and the view is locked.
	ModeScrolled
)

// ScrollbackViewport renders a window into the scrollback buffer.
// It implements windowed rendering for performance - only visible lines are processed.
type ScrollbackViewport struct {
	buffer     *ScrollbackBuffer
	offset     int        // Lines from bottom (0 = showing newest)
	height     int        // Visible rows
	width      int        // Terminal width
	mode       ScrollMode // Live or Scrolled
	newLines   int        // Count of new lines since scrolling back
	cacheValid bool
	cachedView string
	prompt     string // Current server prompt (partial line)
}

// NewScrollbackViewport creates a viewport for the given buffer.
func NewScrollbackViewport(buffer *ScrollbackBuffer) *ScrollbackViewport {
	return &ScrollbackViewport{
		buffer: buffer,
		mode:   ModeLive,
	}
}

// SetDimensions updates the viewport size.
func (v *ScrollbackViewport) SetDimensions(width, height int) {
	if v.width != width || v.height != height {
		v.width = width
		v.height = height
		v.cacheValid = false
	}
}

// OnNewLines is called when new lines are appended to the buffer.
// In live mode, the view auto-scrolls. In scrolled mode, offset is adjusted
// to maintain the user's reading position.
func (v *ScrollbackViewport) OnNewLines(count int) {
	switch v.mode {
	case ModeLive:
		// Stay pinned to bottom - nothing to do
		v.cacheValid = false
	case ModeScrolled:
		// Maintain reading position by increasing offset
		v.offset += count
		v.newLines += count
		v.cacheValid = false
	}
}

// ScrollUp moves the view up (towards older lines).
func (v *ScrollbackViewport) ScrollUp(lines int) {
	maxOffset := v.buffer.Count() - v.height
	if maxOffset < 0 {
		maxOffset = 0
	}

	v.offset += lines
	if v.offset > maxOffset {
		v.offset = maxOffset
	}

	if v.offset > 0 {
		v.mode = ModeScrolled
	}
	v.cacheValid = false
}

// ScrollDown moves the view down (towards newer lines).
func (v *ScrollbackViewport) ScrollDown(lines int) {
	v.offset -= lines
	if v.offset <= 0 {
		v.offset = 0
		v.mode = ModeLive
		v.newLines = 0
	}
	v.cacheValid = false
}

// PageUp scrolls up by one page.
func (v *ScrollbackViewport) PageUp() {
	v.ScrollUp(v.height - 1)
}

// PageDown scrolls down by one page.
func (v *ScrollbackViewport) PageDown() {
	v.ScrollDown(v.height - 1)
}

// GotoBottom returns to live mode (pinned to newest lines).
func (v *ScrollbackViewport) GotoBottom() {
	v.offset = 0
	v.mode = ModeLive
	v.newLines = 0
	v.cacheValid = false
}

// GotoTop scrolls to the oldest line.
func (v *ScrollbackViewport) GotoTop() {
	maxOffset := v.buffer.Count() - v.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	v.offset = maxOffset
	if v.offset > 0 {
		v.mode = ModeScrolled
	}
	v.cacheValid = false
}

// Mode returns the current scroll mode.
func (v *ScrollbackViewport) Mode() ScrollMode {
	return v.mode
}

// NewLineCount returns the number of new lines since the user scrolled back.
func (v *ScrollbackViewport) NewLineCount() int {
	return v.newLines
}

// InvalidateCache forces a re-render on the next View() call.
func (v *ScrollbackViewport) InvalidateCache() {
	v.cacheValid = false
}

// SetPrompt sets the current server prompt (partial line without newline).
// This is displayed at the bottom of the viewport in live mode.
func (v *ScrollbackViewport) SetPrompt(text string) {
	if v.prompt != text {
		v.prompt = text
		v.cacheValid = false
	}
}

// ClearPrompt clears the current prompt (called when a full line arrives).
func (v *ScrollbackViewport) ClearPrompt() {
	if v.prompt != "" {
		v.prompt = ""
		v.cacheValid = false
	}
}

// View renders the visible portion of the scrollback buffer.
// Lines are returned with ANSI codes preserved.
// Always returns exactly height lines (padded with empty lines if needed).
func (v *ScrollbackViewport) View() string {
	if v.cacheValid {
		return v.cachedView
	}

	if v.height <= 0 {
		v.cachedView = ""
		v.cacheValid = true
		return v.cachedView
	}

	var b strings.Builder
	b.Grow(v.height * (v.width + 1)) // Pre-allocate approximate size

	if v.buffer.Count() == 0 {
		// No content - just return empty lines to fill the space
		for i := 0; i < v.height; i++ {
			if i > 0 {
				b.WriteByte('\n')
			}
		}
		v.cachedView = b.String()
		v.cacheValid = true
		return v.cachedView
	}

	// Calculate which lines are visible
	totalLines := v.buffer.Count()

	// endIdx is the index just past the last visible line (exclusive)
	// In live mode (offset=0), endIdx = totalLines (show up to the newest)
	endIdx := totalLines - v.offset
	if endIdx > totalLines {
		endIdx = totalLines
	}

	// startIdx is the first visible line
	startIdx := endIdx - v.height
	if startIdx < 0 {
		startIdx = 0
	}

	// Get the visible lines
	visibleLines := v.buffer.GetRange(startIdx, endIdx)

	// Pad with empty lines at the TOP if we don't have enough content
	// This pushes content to the bottom of the viewport
	emptyLines := v.height - len(visibleLines)
	for i := 0; i < emptyLines; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
	}

	// Write content lines
	for i, line := range visibleLines {
		if emptyLines > 0 || i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}

	// Append server prompt (partial line) at the end in live mode
	if v.mode == ModeLive && v.prompt != "" {
		b.WriteByte('\n')
		b.WriteString(v.prompt)
	}

	v.cachedView = b.String()
	v.cacheValid = true
	return v.cachedView
}

// VisibleLineCount returns how many lines are currently visible.
func (v *ScrollbackViewport) VisibleLineCount() int {
	visible := v.buffer.Count() - v.offset
	if visible > v.height {
		visible = v.height
	}
	if visible < 0 {
		visible = 0
	}
	return visible
}

// AtBottom returns true if the viewport is showing the newest lines.
func (v *ScrollbackViewport) AtBottom() bool {
	return v.offset == 0
}

// AtTop returns true if the viewport is showing the oldest lines.
func (v *ScrollbackViewport) AtTop() bool {
	maxOffset := v.buffer.Count() - v.height
	if maxOffset < 0 {
		return true
	}
	return v.offset >= maxOffset
}
