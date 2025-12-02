package widget

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ScrollbackBuffer is a ring buffer for storing terminal output lines.
type ScrollbackBuffer struct {
	lines    []string
	head     int
	tail     int
	count    int
	capacity int
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

// Append adds a line to the buffer.
func (sb *ScrollbackBuffer) Append(line string) {
	sb.lines[sb.tail] = line
	sb.tail = (sb.tail + 1) % sb.capacity

	if sb.count < sb.capacity {
		sb.count++
	} else {
		sb.head = (sb.head + 1) % sb.capacity
	}
}

// Count returns the number of lines.
func (sb *ScrollbackBuffer) Count() int {
	return sb.count
}

// At retrieves a line by logical index (0 = oldest).
func (sb *ScrollbackBuffer) At(i int) string {
	if i < 0 || i >= sb.count {
		return ""
	}
	actualIndex := (sb.head + i) % sb.capacity
	return sb.lines[actualIndex]
}

// Viewport renders a window into the scrollback buffer.
type Viewport struct {
	buffer     *ScrollbackBuffer
	offset     int        // Lines from bottom (0 = showing newest)
	height     int
	width      int
	mode       ScrollMode
	newLines   int
	cacheValid bool
	cachedView string
	prompt     string
}

// NewViewport creates a viewport for the given buffer.
func NewViewport(buffer *ScrollbackBuffer) *Viewport {
	return &Viewport{
		buffer: buffer,
		mode:   ModeLive,
	}
}

// Init implements tea.Model.
func (v *Viewport) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (v *Viewport) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return v, nil
}

// View implements tea.Model.
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
			b.WriteString(v.prompt)
		}
		v.cachedView = b.String()
		v.cacheValid = true
		return v.cachedView
	}

	totalLines := v.buffer.Count()
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

	for i := startIdx; i < endIdx; i++ {
		if emptyLines > 0 || i > startIdx {
			b.WriteByte('\n')
		}
		b.WriteString(v.buffer.At(i))
	}

	if hasPrompt {
		b.WriteByte('\n')
		b.WriteString(v.prompt)
	}

	v.cachedView = b.String()
	v.cacheValid = true
	return v.cachedView
}

// SetWidth implements Widget.
func (v *Viewport) SetWidth(w int) {
	if w != v.width {
		v.width = w
		v.cacheValid = false
	}
}

// Height implements Widget.
func (v *Viewport) Height() int {
	return v.height
}

// SetDimensions sets viewport size.
func (v *Viewport) SetDimensions(width, height int) {
	if width != v.width || height != v.height {
		v.width = width
		v.height = height
		v.cacheValid = false
	}
}

// OnNewLines is called when lines are added.
func (v *Viewport) OnNewLines(count int) {
	switch v.mode {
	case ModeLive:
		v.cacheValid = false
	case ModeScrolled:
		v.offset += count
		v.newLines += count
		v.cacheValid = false
	}
}

// SetPrompt sets the server prompt.
func (v *Viewport) SetPrompt(text string) {
	if v.prompt != text {
		v.prompt = text
		v.cacheValid = false
	}
}

// PageUp scrolls up one page.
func (v *Viewport) PageUp() {
	maxOffset := v.buffer.Count() - v.height
	if maxOffset < 0 {
		maxOffset = 0
	}

	v.offset += v.height - 1
	if v.offset > maxOffset {
		v.offset = maxOffset
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

// GotoBottom returns to live mode.
func (v *Viewport) GotoBottom() {
	v.offset = 0
	v.mode = ModeLive
	v.newLines = 0
	v.cacheValid = false
}

// GotoTop scrolls to the oldest line.
func (v *Viewport) GotoTop() {
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
func (v *Viewport) Mode() ScrollMode {
	return v.mode
}

// NewLineCount returns lines added while scrolled.
func (v *Viewport) NewLineCount() int {
	return v.newLines
}
