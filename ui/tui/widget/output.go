package widget

import (
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

// ScrollMode indicates whether output is live or scrolled back.
type ScrollMode int

const (
	ModeLive ScrollMode = iota
	ModeScrolled
)

// Output owns the main pane: scrollback, prompt, and scrolling. Scrollback
// holds logical lines; Output wraps the ones it shows at its current width,
// so a width change reflows history.
type Output struct {
	scrollback   *Scrollback
	live         bool   // tailing the newest row; otherwise scrolled to the anchor
	anchorSeq    uint64 // line holding the bottom visible row while scrolled
	anchorCol    int    // column where that row starts within its line
	height       int
	width        int
	newLines     int
	cacheValid   bool
	cachedView   string
	prompt       string
	styles       style.Styles
	hasPlacement bool

	rowMemo   []rowMemo              // wrapped row counts, indexed by seq modulo capacity
	wrapCache map[uint64]wrappedLine // multi-row lines recently drawn at this width

	// Search-match highlight, anchored by sequence number so appends
	// and ring eviction cannot smear it onto a different line.
	hlSet    bool
	hlSeq    uint64
	hlRanges []util.ColRange
}

// rowMemo caches how many rows one line wraps to at one width.
type rowMemo struct {
	seq         uint64
	width, rows int32
}

// wrappedLine is a line split into styled rows, with the line columns
// each row covers.
type wrappedLine struct {
	rows  []string
	spans []util.ColRange
}

// cursor addresses one wrapped row: a scrollback line index and the row
// within that line at the current width. It is never stored across
// width changes; anchorSeq/anchorCol are.
type cursor struct {
	line, row int
}

func (c cursor) before(d cursor) bool {
	return c.line < d.line || (c.line == d.line && c.row < d.row)
}

// NewOutput creates the main output pane with bounded scrollback.
func NewOutput(capacity int, styles style.Styles) *Output {
	sb := NewScrollback(capacity)
	return &Output{
		scrollback: sb,
		live:       true,
		width:      80,
		styles:     styles,
		rowMemo:    make([]rowMemo, sb.capacity),
		wrapCache:  map[uint64]wrappedLine{},
	}
}

func (o *Output) Scrollback() *Scrollback  { return o.scrollback }
func (o *Output) Width() int               { return o.width }
func (o *Output) Prompt() string           { return o.prompt }
func (o *Output) Title() string            { return "" }
func (o *Output) MinimumSize() image.Point { return image.Pt(0, 1) }

// MeasureHeight counts wrapped rows at width only until the layout's height budget is met.
func (o *Output) MeasureHeight(width, limit int) int {
	rows := 0
	if o.prompt != "" {
		rows++
	}
	for i := o.scrollback.Count() - 1; i >= 0 && rows < limit; i-- {
		rows += measureRows(o.scrollback.At(i), width)
	}
	return min(rows, limit)
}

// measureRows counts the rows line wraps to at width. The byte-length
// check is a fast bound: a rune's display width never exceeds its byte count.
func measureRows(line string, width int) int {
	if width < 1 || len(line) <= width || ansi.StringWidth(line) <= width {
		return 1
	}
	return len(util.WrapLine(line, width))
}

// Write appends logical lines. Wrapping happens at render time.
func (o *Output) Write(text string) {
	if !strings.ContainsAny(text, "\r\n\t") {
		o.scrollback.Append(text)
		o.onNewLines(1)
		return
	}
	lines := util.SplitLines(text)
	for _, line := range lines {
		o.scrollback.Append(util.ExpandTabs(line))
	}
	o.onNewLines(len(lines))
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
// Hiding a previously placed pane retains its last placement geometry.
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

	bottom, live := o.resolve()
	hasPrompt := live && o.prompt != ""
	contentHeight := o.height
	if hasPrompt {
		contentHeight--
	}

	// Fill upward from the bottom row; rows left empty pad the top.
	rows := make([]string, contentHeight)
	y := contentHeight - 1
	for line := bottom.line; line >= 0 && y >= 0; line-- {
		last := bottom.row
		if line != bottom.line {
			last = o.rows(line) - 1
		}
		take := min(last+1, y+1)
		o.renderRows(line, last+1-take, rows[y+1-take:y+1])
		y -= take
	}

	var b strings.Builder
	b.Grow(o.height * (o.width + 1))
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
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

// renderRows fills dst with the wrapped rows of line index i starting at row first.
func (o *Output) renderRows(i, first int, dst []string) {
	if o.rows(i) > 1 {
		copy(dst, o.wrapped(i).rows[first:])
		return
	}
	line := o.scrollback.At(i)
	if o.hlSet && o.scrollback.Seq(i) == o.hlSeq {
		line = o.highlight(line)
	}
	dst[0] = line
}

func (o *Output) highlight(line string) string {
	return util.HighlightRanges(line, o.hlRanges, func(s string) string { return o.styles.OverlayMatch.Render(s) })
}

// rows returns how many rows line index i wraps to at the current width.
func (o *Output) rows(i int) int {
	line := o.scrollback.At(i)
	if o.width < 1 || len(line) <= o.width {
		return 1
	}
	seq := o.scrollback.Seq(i)
	memo := &o.rowMemo[seq%uint64(len(o.rowMemo))]
	if memo.rows == 0 || memo.seq != seq || int(memo.width) != o.width {
		*memo = rowMemo{seq: seq, width: int32(o.width), rows: int32(measureRows(line, o.width))}
	}
	return int(memo.rows)
}

// wrapped splits line index i into rows at the current width. Lines are
// cached while they stay on screen, so redrawing a very long line does
// not rewrap it every frame.
func (o *Output) wrapped(i int) wrappedLine {
	seq := o.scrollback.Seq(i)
	if w, ok := o.wrapCache[seq]; ok {
		return w
	}
	if len(o.wrapCache) > 4*max(1, o.height) {
		clear(o.wrapCache)
	}
	line := o.scrollback.At(i)
	if o.hlSet && seq == o.hlSeq {
		// Highlighting preserves visible text, so wrap points do not move.
		line = o.highlight(line)
	}
	rows := util.WrapLine(line, o.width)
	w := wrappedLine{spans: util.WrapSpans(line, rows), rows: util.CarrySGR(rows)}
	o.wrapCache[seq] = w
	return w
}

// rowAt returns the row of line index i that contains column col.
func (o *Output) rowAt(i, col int) int {
	if col <= 0 || o.rows(i) == 1 {
		return 0
	}
	row := 0
	for k, span := range o.wrapped(i).spans {
		if span.Start <= col {
			row = k
		}
	}
	return row
}

// colOf returns the line column where row c starts.
func (o *Output) colOf(c cursor) int {
	if c.row == 0 {
		return 0
	}
	return o.wrapped(c.line).spans[c.row].Start
}

// newest is the last row of the newest line.
func (o *Output) newest() cursor {
	n := o.scrollback.Count() - 1
	return cursor{n, o.rows(n) - 1}
}

// up moves c toward older rows by n, stopping at the oldest row.
func (o *Output) up(c cursor, n int) cursor {
	for n > 0 {
		if c.row >= n {
			c.row -= n
			break
		}
		n -= c.row
		c.row = 0
		if c.line == 0 {
			break
		}
		c.line--
		c.row = o.rows(c.line) - 1
		n--
	}
	return c
}

// down moves c toward newer rows by n, stopping at the newest row.
func (o *Output) down(c cursor, n int) cursor {
	last := o.scrollback.Count() - 1
	for n > 0 {
		rest := o.rows(c.line) - 1 - c.row
		if rest >= n {
			c.row += n
			break
		}
		c.row += rest
		n -= rest
		if c.line == last {
			break
		}
		c.line++
		c.row = 0
		n--
	}
	return c
}

// clamp keeps a scrolled bottom row low enough that the window stays
// full, and reports live once it reaches the newest row.
func (o *Output) clamp(c cursor) (cursor, bool) {
	if o.scrollback.Count() == 0 {
		return cursor{}, true
	}
	if top := o.down(cursor{}, o.height-1); c.before(top) {
		c = top
	}
	newest := o.newest()
	if !c.before(newest) {
		return newest, true
	}
	return c, false
}

// resolve maps the stored anchor onto the current width. An evicted
// anchor pins to the oldest full window.
func (o *Output) resolve() (cursor, bool) {
	if o.scrollback.Count() == 0 {
		return cursor{}, true
	}
	if o.live {
		return o.newest(), true
	}
	return o.clamp(o.locate(o.anchorSeq, o.anchorCol))
}

// locate finds the row holding column col of the line with sequence seq,
// or the oldest row when that line has been evicted.
func (o *Output) locate(seq uint64, col int) cursor {
	if idx, ok := o.scrollback.IndexOf(seq); ok {
		return cursor{idx, o.rowAt(idx, col)}
	}
	return cursor{}
}

// setCursor makes c the bottom visible row, or returns to live at the newest row.
func (o *Output) setCursor(c cursor) {
	c, live := o.clamp(c)
	o.live = live
	if live {
		o.newLines = 0
	} else {
		o.anchorSeq = o.scrollback.Seq(c.line)
		o.anchorCol = o.colOf(c)
	}
	o.cacheValid = false
}

// SetSize applies output window dimensions and clamps its scroll position.
func (o *Output) SetSize(width, height int) {
	o.hasPlacement = true
	o.resize(width, height)
}

// resize keeps the same text at the bottom of a scrolled window: the
// anchor names a line and column, which survive rewrapping.
func (o *Output) resize(width, height int) {
	if width != o.width || height != o.height {
		if width != o.width {
			clear(o.wrapCache)
		}
		o.width = width
		o.height = height
		if !o.live {
			o.setCursor(o.locate(o.anchorSeq, o.anchorCol))
		}
		o.cacheValid = false
	}
}

// onNewLines is called when lines are appended to the scrollback. A
// scrolled window stays anchored to its line.
func (o *Output) onNewLines(count int) {
	if !o.live {
		o.newLines += count
	}
	o.cacheValid = false
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
	if rows := o.height - 1; rows > 0 {
		o.ScrollUp(rows)
	}
}

// PageDown scrolls down one page.
func (o *Output) PageDown() {
	if rows := o.height - 1; rows > 0 {
		o.ScrollDown(rows)
	}
}

// ScrollUp scrolls up by the given rows (toward older content).
func (o *Output) ScrollUp(rows int) {
	if rows <= 0 || o.scrollback.Count() == 0 {
		return
	}
	c, _ := o.resolve()
	o.setCursor(o.up(c, rows))
}

// ScrollDown scrolls down by the given rows (toward newer content).
func (o *Output) ScrollDown(rows int) {
	if rows <= 0 || o.live {
		return
	}
	c, _ := o.resolve()
	o.setCursor(o.down(c, rows))
}

// ScrollToBottom returns to live mode.
func (o *Output) ScrollToBottom() {
	o.live = true
	o.newLines = 0
	o.cacheValid = false
}

// ScrollToTop scrolls to the oldest line.
func (o *Output) ScrollToTop() {
	if o.scrollback.Count() == 0 {
		return
	}
	o.setCursor(cursor{})
}

// CenterOn scrolls so the row holding column col of the line with the
// given sequence number sits vertically centered (as close as clamping
// allows). An evicted line pins to the oldest surviving window.
func (o *Output) CenterOn(seq uint64, col int) {
	if o.scrollback.Count() == 0 {
		return
	}
	o.setCursor(o.down(o.locate(seq, col), (o.height+1)/2-1))
}

// SetHighlight marks visible column ranges of the line with the given
// sequence number to render restyled (search-match highlighting).
func (o *Output) SetHighlight(seq uint64, ranges []util.ColRange) {
	o.hlSet = true
	o.hlSeq = seq
	o.hlRanges = ranges
	clear(o.wrapCache)
	o.cacheValid = false
}

// ClearHighlight removes the search-match highlight.
func (o *Output) ClearHighlight() {
	if o.hlSet {
		o.hlSet = false
		o.hlRanges = nil
		clear(o.wrapCache)
		o.cacheValid = false
	}
}

// Clear empties scrollback and resets scrolling and highlighting.
// The live prompt remains visible.
func (o *Output) Clear() {
	o.scrollback.Clear()
	clear(o.wrapCache)
	o.live = true
	o.newLines = 0
	o.hlSet = false
	o.hlRanges = nil
	o.cacheValid = false
}

// ScrollPos is a sequence-anchored snapshot of the output window position.
// It names the bottom visible row by line and starting column, which
// survive both appends and rewrapping at a new width.
type ScrollPos struct {
	BottomSeq uint64 // sequence of the line holding the bottom visible row
	BottomCol int    // column where that row starts within its line
	Mode      ScrollMode
	NewLines  int
	Appended  uint64 // scrollback append counter at save time
}

// SaveScroll captures the current position for a later RestoreScroll.
func (o *Output) SaveScroll() ScrollPos {
	p := ScrollPos{Mode: o.Mode(), NewLines: o.newLines, Appended: o.scrollback.appended}
	if o.scrollback.Count() > 0 {
		c, _ := o.resolve()
		p.BottomSeq = o.scrollback.Seq(c.line)
		p.BottomCol = o.colOf(c)
	}
	return p
}

// RestoreScroll returns to a saved position: the same text, not the
// same distance from live. A live snapshot returns to live (tailing is
// itself a position); a scrolled snapshot re-anchors on the saved
// bottom row, counting lines that arrived meanwhile into NewLineCount,
// and pins to the oldest surviving window if the line was evicted.
func (o *Output) RestoreScroll(p ScrollPos) {
	if p.Mode == ModeLive || o.scrollback.Count() == 0 {
		o.ScrollToBottom()
		return
	}
	o.setCursor(o.locate(p.BottomSeq, p.BottomCol))
	if !o.live {
		o.newLines = p.NewLines + int(o.scrollback.appended-p.Appended)
	}
}

// Mode returns the current scroll mode.
func (o *Output) Mode() ScrollMode {
	if o.live {
		return ModeLive
	}
	return ModeScrolled
}

// NewLineCount returns lines added while scrolled.
func (o *Output) NewLineCount() int {
	return o.newLines
}
