package widget

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// clipRow hard-truncates one row to the given width (ANSI-aware).
// Every widget row must fit the terminal width: an overlong row wraps
// at the terminal, adds a phantom physical line, and scrolls the whole
// frame - corrupting the layout for everything above it.
func clipRow(s string, width int) string {
	if width < 1 || util.VisibleLen(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "")
}

// Compile-time check that Viewport implements Widget
var _ Widget = (*Viewport)(nil)

// ScrollMode indicates whether viewport is live or scrolled back.
type ScrollMode int

const (
	ModeLive ScrollMode = iota
	ModeScrolled
)

// ScrollbackBuffer is a ring buffer of physical rows of terminal
// output; each entry renders as exactly one row.
type ScrollbackBuffer struct {
	lines    []string
	head     int
	tail     int
	count    int
	capacity int
	appended uint64 // rows ever appended; assigns eviction-stable sequence numbers
}

// NewScrollbackBuffer creates a new ring buffer.
func NewScrollbackBuffer(capacity int) *ScrollbackBuffer {
	if capacity <= 0 {
		capacity = 100000
	}
	return &ScrollbackBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Append adds a row to the buffer.
func (sb *ScrollbackBuffer) Append(row string) {
	sb.lines[sb.tail] = row
	sb.tail = (sb.tail + 1) % sb.capacity
	sb.appended++

	if sb.count < sb.capacity {
		sb.count++
	} else {
		sb.head = (sb.head + 1) % sb.capacity
	}
}

// Count returns the number of rows.
func (sb *ScrollbackBuffer) Count() int {
	return sb.count
}

// At retrieves a row by index (0 = oldest).
func (sb *ScrollbackBuffer) At(i int) string {
	if i < 0 || i >= sb.count {
		return ""
	}
	actualIndex := (sb.head + i) % sb.capacity
	return sb.lines[actualIndex]
}

// Seq returns the absolute sequence number of the row at index i.
// Indices shift as the full ring evicts old rows; sequence numbers
// never do, so they can anchor positions across appends.
func (sb *ScrollbackBuffer) Seq(i int) uint64 {
	return sb.appended - uint64(sb.count) + uint64(i)
}

// IndexOf maps an absolute sequence number back to a current index;
// ok is false when that row has been evicted (or never existed).
func (sb *ScrollbackBuffer) IndexOf(seq uint64) (int, bool) {
	oldest := sb.appended - uint64(sb.count)
	if seq < oldest || seq >= sb.appended {
		return 0, false
	}
	return int(seq - oldest), true
}

// Viewport renders a window into the scrollback buffer.
type Viewport struct {
	buffer     *ScrollbackBuffer
	offset     int // Lines from bottom (0 = showing newest)
	height     int
	width      int
	mode       ScrollMode
	newLines   int
	cacheValid bool
	cachedView string
	prompt     string
	styles     style.Styles

	// Search-match highlight, anchored by sequence number so appends
	// and ring eviction cannot smear it onto a different row.
	hlSet    bool
	hlSeq    uint64
	hlRanges []util.ColRange
}

// NewViewport creates a viewport for the given buffer.
func NewViewport(buffer *ScrollbackBuffer, styles style.Styles) *Viewport {
	return &Viewport{
		buffer: buffer,
		mode:   ModeLive,
		styles: styles,
	}
}

// View implements Widget.
func (v *Viewport) View() string {
	if v.cacheValid {
		return v.cachedView
	}

	if v.height <= 0 {
		v.cachedView = ""
		v.cacheValid = true
		return v.cachedView
	}

	var b strings.Builder
	b.Grow(v.height * (v.width + 1))

	hasPrompt := v.mode == ModeLive && v.prompt != ""
	contentHeight := v.height
	if hasPrompt {
		contentHeight--
	}

	if v.buffer.Count() == 0 {
		for i := 0; i < contentHeight; i++ {
			if i > 0 {
				b.WriteByte('\n')
			}
		}
		if hasPrompt {
			if contentHeight > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(clipRow(v.prompt, v.width))
		}
		v.cachedView = b.String()
		v.cacheValid = true
		return v.cachedView
	}

	// Defensive: whatever happened to the offset, the frame must never
	// grow taller than the assigned height. An offset beyond Count()
	// would make endIdx negative and the padding loop below emit more
	// than contentHeight rows.
	totalLines := v.buffer.Count()
	if v.offset > totalLines {
		v.offset = totalLines
	}
	endIdx := totalLines - v.offset
	if endIdx > totalLines {
		endIdx = totalLines
	}

	startIdx := endIdx - contentHeight
	if startIdx < 0 {
		startIdx = 0
	}

	visibleCount := endIdx - startIdx
	emptyLines := contentHeight - visibleCount

	for i := 0; i < emptyLines; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
	}

	hlIdx := -1
	if v.hlSet {
		if idx, ok := v.buffer.IndexOf(v.hlSeq); ok {
			hlIdx = idx
		}
	}

	for i := startIdx; i < endIdx; i++ {
		if emptyLines > 0 || i > startIdx {
			b.WriteByte('\n')
		}
		row := v.buffer.At(i)
		if i == hlIdx {
			// Splice first, clip second: highlighting must not let a
			// row exceed the terminal width.
			row = util.HighlightRanges(row, v.hlRanges, func(s string) string {
				return v.styles.OverlayMatch.Render(s)
			})
		}
		b.WriteString(clipRow(row, v.width))
	}

	if hasPrompt {
		b.WriteByte('\n')
		b.WriteString(clipRow(v.prompt, v.width))
	}

	v.cachedView = b.String()
	v.cacheValid = true
	return v.cachedView
}

// SetSize implements Widget.
func (v *Viewport) SetSize(width, height int) {
	if width != v.width || height != v.height {
		v.width = width
		v.height = height
		v.cacheValid = false
	}
}

// PreferredHeight implements Widget.
// Viewport is a fill component - it takes whatever space is allocated.
func (v *Viewport) PreferredHeight() int {
	return v.height
}

// OnNewRows is called when rows are appended to the buffer.
func (v *Viewport) OnNewRows(count int) {
	switch v.mode {
	case ModeLive:
		v.cacheValid = false
	case ModeScrolled:
		v.offset += count
		v.newLines += count
		// Once the ring buffer is full, appends evict the oldest rows
		// and Count() stops growing - the rows this offset was anchored
		// on may be gone. Pin to the oldest surviving window (like
		// Pane.clampOffset) instead of letting the offset drift past
		// the buffer and inflate the rendered frame.
		if max := v.maxOffset(); v.offset > max {
			v.offset = max
		}
		v.cacheValid = false
	}
}

// maxOffset is the largest scroll offset that still fills the window
// with buffered rows; 0 when the buffer fits the viewport.
func (v *Viewport) maxOffset() int {
	max := v.buffer.Count() - v.height
	if max < 0 {
		max = 0
	}
	return max
}

// SetPrompt replaces the prompt overlay.
func (v *Viewport) SetPrompt(text string) {
	if v.prompt != text {
		v.prompt = text
		v.cacheValid = false
	}
}

// PageUp scrolls up one page.
func (v *Viewport) PageUp() {
	v.offset += v.height - 1
	if max := v.maxOffset(); v.offset > max {
		v.offset = max
	}

	if v.offset > 0 {
		v.mode = ModeScrolled
	}
	v.cacheValid = false
}

// PageDown scrolls down one page.
func (v *Viewport) PageDown() {
	v.offset -= v.height - 1
	if v.offset <= 0 {
		v.offset = 0
		v.mode = ModeLive
		v.newLines = 0
	}
	v.cacheValid = false
}

// ScrollUp scrolls up by N lines (toward older content).
func (v *Viewport) ScrollUp(lines int) {
	v.offset += lines
	if max := v.maxOffset(); v.offset > max {
		v.offset = max
	}

	if v.offset > 0 {
		v.mode = ModeScrolled
	}
	v.cacheValid = false
}

// ScrollDown scrolls down by N lines (toward newer content).
func (v *Viewport) ScrollDown(lines int) {
	v.offset -= lines
	if v.offset <= 0 {
		v.offset = 0
		v.mode = ModeLive
		v.newLines = 0
	}
	v.cacheValid = false
}

// GotoBottom returns to live mode.
func (v *Viewport) GotoBottom() {
	v.offset = 0
	v.mode = ModeLive
	v.newLines = 0
	v.cacheValid = false
}

// GotoTop scrolls to the oldest line.
func (v *Viewport) GotoTop() {
	v.offset = v.maxOffset()
	if v.offset > 0 {
		v.mode = ModeScrolled
	}
	v.cacheValid = false
}

// CenterOn scrolls so the row with the given sequence number sits
// vertically centered (as close as clamping allows). An evicted row
// pins to the oldest surviving window, like OnNewRows.
func (v *Viewport) CenterOn(seq uint64) {
	idx, ok := v.buffer.IndexOf(seq)
	if !ok {
		v.offset = v.maxOffset()
	} else {
		v.offset = v.buffer.Count() - idx - (v.height+1)/2
		if max := v.maxOffset(); v.offset > max {
			v.offset = max
		}
		if v.offset < 0 {
			v.offset = 0
		}
	}
	if v.offset > 0 {
		v.mode = ModeScrolled
	} else {
		v.mode = ModeLive
		v.newLines = 0
	}
	v.cacheValid = false
}

// SetHighlight marks visible column ranges of the row with the given
// sequence number to render restyled (search-match highlighting).
func (v *Viewport) SetHighlight(seq uint64, ranges []util.ColRange) {
	v.hlSet = true
	v.hlSeq = seq
	v.hlRanges = ranges
	v.cacheValid = false
}

// ClearHighlight removes the search-match highlight.
func (v *Viewport) ClearHighlight() {
	if v.hlSet {
		v.hlSet = false
		v.hlRanges = nil
		v.cacheValid = false
	}
}

// ScrollPos is a sequence-anchored snapshot of the viewport position.
// A raw offset would not survive appends: OnNewRows re-anchors a
// scrolled viewport's offset on every append, and a snapshot taken
// earlier would land newer by exactly the rows that arrived since.
type ScrollPos struct {
	BottomSeq uint64 // sequence of the bottom visible row at save time
	Mode      ScrollMode
	NewLines  int
	Appended  uint64 // buffer append counter at save time
}

// SaveScroll captures the current position for a later RestoreScroll.
func (v *Viewport) SaveScroll() ScrollPos {
	p := ScrollPos{Mode: v.mode, NewLines: v.newLines, Appended: v.buffer.appended}
	if c := v.buffer.Count(); c > 0 {
		bottom := c - 1 - v.offset
		if bottom < 0 {
			bottom = 0
		}
		p.BottomSeq = v.buffer.Seq(bottom)
	}
	return p
}

// RestoreScroll returns to a saved position: the same text, not the
// same distance from live. A live snapshot returns to live (tailing is
// itself a position); a scrolled snapshot re-anchors on the saved
// bottom row, counting rows that arrived meanwhile into NewLineCount,
// and pins to the oldest surviving window if the row was evicted.
func (v *Viewport) RestoreScroll(p ScrollPos) {
	if p.Mode == ModeLive {
		v.GotoBottom()
		return
	}
	if idx, ok := v.buffer.IndexOf(p.BottomSeq); ok {
		v.offset = v.buffer.Count() - 1 - idx
	} else {
		v.offset = v.maxOffset()
	}
	if max := v.maxOffset(); v.offset > max {
		v.offset = max
	}
	if v.offset <= 0 {
		v.offset = 0
		v.mode = ModeLive
		v.newLines = 0
	} else {
		v.mode = ModeScrolled
		v.newLines = p.NewLines + int(v.buffer.appended-p.Appended)
	}
	v.cacheValid = false
}

// Mode returns the current scroll mode.
func (v *Viewport) Mode() ScrollMode {
	return v.mode
}

// NewLineCount returns lines added while scrolled.
func (v *Viewport) NewLineCount() int {
	return v.newLines
}
