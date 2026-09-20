package widget

import (
	"image"
	"strings"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// ScrollMode indicates whether output is live or scrolled back.
type ScrollMode int

const (
	ModeLive ScrollMode = iota
	ModeScrolled
)

// Output owns the main pane: append-time wrapping, scrollback, prompt, and scrolling.
type Output struct {
	scrollback   *Scrollback
	offset       int // Lines from bottom (0 = showing newest)
	height       int
	width        int
	newLines     int
	cacheValid   bool
	cachedView   string
	prompt       string
	styles       style.Styles
	hasPlacement bool

	// Search-match highlight, anchored by sequence number so appends
	// and ring eviction cannot smear it onto a different row.
	hlSet    bool
	hlSeq    uint64
	hlRanges []util.ColRange
}

// NewOutput creates the main output pane with bounded scrollback.
func NewOutput(capacity int, styles style.Styles) *Output {
	return &Output{scrollback: NewScrollback(capacity), width: 80, styles: styles}
}

func (o *Output) Scrollback() *Scrollback  { return o.scrollback }
func (o *Output) Width() int               { return o.width }
func (o *Output) Prompt() string           { return o.prompt }
func (o *Output) Name() string             { return ui.OutputPaneName }
func (o *Output) Title() string            { return "" }
func (o *Output) MinimumSize() image.Point { return image.Pt(0, 1) }

func (o *Output) MeasureHeight(width, limit int) int {
	rows := o.scrollback.Count()
	if o.prompt != "" {
		rows++
	}
	return min(rows, limit)
}

// Write fixes physical rows at the current width. Resizing never reflows history.
func (o *Output) Write(text string) {
	// Byte length bounds display width. Most incoming rows can be retained
	// directly, without allocating a slice or parsing ANSI/Unicode twice.
	if len(text) <= o.width && !strings.ContainsAny(text, "\r\n\t") {
		o.scrollback.Append(text)
		o.onNewRows(1)
		return
	}
	count := 0
	for _, line := range util.SplitLines(text) {
		for _, row := range util.WrapLine(util.ExpandTabs(line), o.width) {
			o.scrollback.Append(row)
			count++
		}
	}
	o.onNewRows(count)
}

func (o *Output) CommitPrompt(text string) bool {
	changed := text != "" || o.prompt != ""
	if text != "" {
		o.Write(text)
	}
	o.SetPrompt("")
	return changed
}

// SetFallbackSize supplies initial geometry until output has a real placement.
// Hiding a previously placed pane retains its wrapping and search geometry.
func (o *Output) SetFallbackSize(width, height int) {
	if !o.hasPlacement && width > 0 {
		o.resize(width, max(1, height))
	}
}

// View renders and caches the current scrollback window without changing scroll state.
func (o *Output) View() string {
	if o.cacheValid {
		return o.cachedView
	}

	if o.height <= 0 {
		o.cachedView = ""
		o.cacheValid = true
		return o.cachedView
	}

	var b strings.Builder
	b.Grow(o.height * (o.width + 1))

	hasPrompt := o.offset == 0 && o.prompt != ""
	contentHeight := o.height
	if hasPrompt {
		contentHeight--
	}

	// Sequence anchors live in the scrollback; only this window's indices are relative.
	end := max(0, o.scrollback.Count()-o.offset)
	start := max(0, end-contentHeight)
	padding := contentHeight - (end - start)
	highlight := -1
	if o.hlSet {
		if index, ok := o.scrollback.IndexOf(o.hlSeq); ok {
			highlight = index
		}
	}
	for y := 0; y < contentHeight; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		if y < padding {
			continue
		}
		index := start + y - padding
		row := o.scrollback.At(index)
		if index == highlight {
			row = util.HighlightRanges(row, o.hlRanges, func(s string) string { return o.styles.OverlayMatch.Render(s) })
		}
		b.WriteString(util.ClipRow(row, o.width))
	}
	if hasPrompt {
		if contentHeight > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(util.ClipRow(o.prompt, o.width))
	}
	o.cachedView = b.String()
	o.cacheValid = true
	return o.cachedView
}

// SetSize applies output window dimensions and clamps its scroll position.
func (o *Output) SetSize(width, height int) {
	o.hasPlacement = true
	o.resize(width, height)
}

func (o *Output) resize(width, height int) {
	if width != o.width || height != o.height {
		o.width = width
		o.height = height
		o.clampOffset()
		o.cacheValid = false
	}
}

// onNewRows is called when rows are appended to the scrollback.
func (o *Output) onNewRows(count int) {
	if o.offset > 0 {
		maxOffset := o.maxOffset()
		if count >= maxOffset-o.offset {
			o.offset = maxOffset
		} else {
			o.offset += count
		}
		o.newLines += count
	}
	o.cacheValid = false
}

// maxOffset is the largest scroll offset that still fills the window
// with buffered rows; 0 when the scrollback fits the output window.
func (o *Output) maxOffset() int {
	return max(0, o.scrollback.Count()-o.height)
}

func (o *Output) clampOffset() {
	if o.offset < 0 {
		o.offset = 0
	}
	if maxOffset := o.maxOffset(); o.offset > maxOffset {
		o.offset = maxOffset
	}
	if o.offset == 0 {
		o.newLines = 0
	}
}

// SetPrompt replaces the prompt overlay.
func (o *Output) SetPrompt(text string) bool {
	text = util.ExpandTabs(text)
	if o.prompt == text {
		return false
	}
	o.prompt = text
	o.cacheValid = false
	return true
}

// PageUp scrolls up one page.
func (o *Output) PageUp() {
	if lines := o.height - 1; lines > 0 {
		o.ScrollUp(lines)
	}
}

// PageDown scrolls down one page.
func (o *Output) PageDown() {
	if lines := o.height - 1; lines > 0 {
		o.ScrollDown(lines)
	}
}

// ScrollUp scrolls up by N lines (toward older content).
func (o *Output) ScrollUp(lines int) {
	if lines <= 0 {
		return
	}
	maxOffset := o.maxOffset()
	if lines >= maxOffset-o.offset {
		o.offset = maxOffset
	} else {
		o.offset += lines
	}
	o.clampOffset()
	o.cacheValid = false
}

// ScrollDown scrolls down by N lines (toward newer content).
func (o *Output) ScrollDown(lines int) {
	if lines <= 0 {
		return
	}
	if lines >= o.offset {
		o.offset = 0
	} else {
		o.offset -= lines
	}
	o.clampOffset()
	o.cacheValid = false
}

// ScrollToBottom returns to live mode.
func (o *Output) ScrollToBottom() {
	o.offset = 0
	o.newLines = 0
	o.cacheValid = false
}

// ScrollToTop scrolls to the oldest line.
func (o *Output) ScrollToTop() {
	o.offset = o.maxOffset()
	o.clampOffset()
	o.cacheValid = false
}

// CenterOn scrolls so the row with the given sequence number sits
// vertically centered (as close as clamping allows). An evicted row
// pins to the oldest surviving window, like onNewRows.
func (o *Output) CenterOn(seq uint64) {
	idx, ok := o.scrollback.IndexOf(seq)
	if !ok {
		o.offset = o.maxOffset()
	} else {
		o.offset = o.scrollback.Count() - idx - (o.height+1)/2
	}
	o.clampOffset()
	o.cacheValid = false
}

// SetHighlight marks visible column ranges of the row with the given
// sequence number to render restyled (search-match highlighting).
func (o *Output) SetHighlight(seq uint64, ranges []util.ColRange) {
	o.hlSet = true
	o.hlSeq = seq
	o.hlRanges = ranges
	o.cacheValid = false
}

// ClearHighlight removes the search-match highlight.
func (o *Output) ClearHighlight() {
	if o.hlSet {
		o.hlSet = false
		o.hlRanges = nil
		o.cacheValid = false
	}
}

// Clear empties scrollback and resets scrolling and highlighting.
// The live prompt remains visible.
func (o *Output) Clear() {
	o.scrollback.Clear()
	o.offset = 0
	o.newLines = 0
	o.hlSet = false
	o.hlRanges = nil
	o.cacheValid = false
}

// ScrollPos is a sequence-anchored snapshot of the output window position.
// A raw offset would not survive appends: onNewRows re-anchors a
// scrolled output window's offset on every append, and a snapshot taken
// earlier would land newer by exactly the rows that arrived since.
type ScrollPos struct {
	BottomSeq uint64 // sequence of the bottom visible row at save time
	Mode      ScrollMode
	NewLines  int
	Appended  uint64 // scrollback append counter at save time
}

// SaveScroll captures the current position for a later RestoreScroll.
func (o *Output) SaveScroll() ScrollPos {
	p := ScrollPos{Mode: o.Mode(), NewLines: o.newLines, Appended: o.scrollback.appended}
	if c := o.scrollback.Count(); c > 0 {
		bottom := c - 1 - o.offset
		if bottom < 0 {
			bottom = 0
		}
		p.BottomSeq = o.scrollback.Seq(bottom)
	}
	return p
}

// RestoreScroll returns to a saved position: the same text, not the
// same distance from live. A live snapshot returns to live (tailing is
// itself a position); a scrolled snapshot re-anchors on the saved
// bottom row, counting rows that arrived meanwhile into NewLineCount,
// and pins to the oldest surviving window if the row was evicted.
func (o *Output) RestoreScroll(p ScrollPos) {
	if p.Mode == ModeLive {
		o.ScrollToBottom()
		return
	}
	if idx, ok := o.scrollback.IndexOf(p.BottomSeq); ok {
		o.offset = o.scrollback.Count() - 1 - idx
	} else {
		o.offset = o.maxOffset()
	}
	o.newLines = p.NewLines + int(o.scrollback.appended-p.Appended)
	o.clampOffset()
	o.cacheValid = false
}

// Mode returns the current scroll mode.
func (o *Output) Mode() ScrollMode {
	if o.offset > 0 {
		return ModeScrolled
	}
	return ModeLive
}

// NewLineCount returns lines added while scrolled.
func (o *Output) NewLineCount() int {
	return o.newLines
}
