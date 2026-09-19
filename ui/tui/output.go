package tui

import (
	"image"
	"strings"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
	"github.com/mmcdole/rune/ui/tui/widget"
)

const defaultOutputWrapWidth = 80

// outputController is Rune's pre-created output pane plus the capabilities
// specific to a MUD transcript: physical-row history, live prompts, search
// anchors, and a retained append-time wrapping width.
type outputController struct {
	buffer   *widget.ScrollbackBuffer
	viewport *widget.Viewport

	wrapWidth    int
	hasPlacement bool
	promptText   string
}

func newOutputController(styles style.Styles) *outputController {
	buffer := widget.NewScrollbackBuffer(100000)
	return &outputController{
		buffer:    buffer,
		viewport:  widget.NewViewport(buffer, styles),
		wrapWidth: defaultOutputWrapWidth,
	}
}

func (o *outputController) Name() string { return ui.OutputPaneName }

func (o *outputController) Write(text string) {
	rows := splitRows(text, o.wrapWidth)
	for _, row := range rows {
		o.buffer.Append(row)
	}
	o.viewport.OnNewRows(len(rows))
}

func (o *outputController) Clear() {
	o.buffer.Clear()
	o.viewport.Clear()
}

func (o *outputController) ScrollUp(lines int)   { o.viewport.ScrollUp(lines) }
func (o *outputController) ScrollDown(lines int) { o.viewport.ScrollDown(lines) }
func (o *outputController) ScrollToTop()         { o.viewport.GotoTop() }
func (o *outputController) ScrollToBottom()      { o.viewport.GotoBottom() }

func (o *outputController) Title() string {
	// Output is untitled by default; layout placements may supply an explicit title.
	return ""
}

func (o *outputController) View() string {
	return o.viewport.View()
}

func (o *outputController) SetSize(width, height int) {
	if width > 0 {
		o.wrapWidth = width
	}
	o.hasPlacement = true
	o.viewport.SetSize(max(1, width), max(1, height))
}

func (o *outputController) MinimumSize() image.Point { return image.Pt(0, 1) }

func (o *outputController) MeasureHeight(width, limit int) int {
	rows := o.buffer.Count()
	if o.promptText != "" {
		rows++
	}
	return min(rows, limit)
}

// setFallbackGeometry gives an output pane that has never been placed a
// useful append/search width. Once it has had real placement geometry, hiding
// or omitting it preserves that geometry so incoming history does not reflow.
func (o *outputController) setFallbackGeometry(width, height int) {
	if o.hasPlacement || width <= 0 {
		return
	}
	o.wrapWidth = width
	o.viewport.SetSize(width, max(1, height))
}

func (o *outputController) setPrompt(text string) {
	text = util.ExpandTabs(text)
	if text != o.promptText {
		o.viewport.SetPrompt(text)
		o.promptText = text
	}
}

func (o *outputController) commitPrompt(text string) {
	if text := util.ExpandTabs(text); text != "" {
		o.Write(text)
	}
	o.viewport.SetPrompt("")
	o.promptText = ""
}

// splitRows shapes a message into physical scrollback rows: one row
// per line break, tabs expanded per row so columns restart on every
// row, and rows wider than the resolved output surface are word-wrapped. Rows
// are final at append time; a resize does not rewrap old output.
func splitRows(msg string, width int) []string {
	if !strings.ContainsAny(msg, "\r\n") {
		return util.WrapLine(util.ExpandTabs(msg), width)
	}
	var rows []string
	for _, line := range util.SplitLines(msg) {
		rows = append(rows, util.WrapLine(util.ExpandTabs(line), width)...)
	}
	return rows
}
