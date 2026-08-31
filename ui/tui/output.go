package tui

import (
	"fmt"

	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
	"github.com/mmcdole/rune/ui/tui/widget"
)

const defaultOutputWrapWidth = 80

// outputController is Rune's pre-created output pane plus the capabilities
// specific to a MUD transcript: physical-row history, batching, live prompts,
// search anchors, and a retained append-time wrapping width.
type outputController struct {
	buffer   *widget.ScrollbackBuffer
	viewport *widget.Viewport

	wrapWidth       int
	hasPlacement    bool
	promptText      string
	pendingRows     []string
	flushScheduled  bool
	batchGeneration uint64
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
	o.flushPending()
	o.appendMessage(text)
}

func (o *outputController) Clear() {
	o.pendingRows = nil
	o.flushScheduled = false
	o.batchGeneration++
	o.buffer.Clear()
	o.viewport.Clear()
}

func (o *outputController) ScrollUp(lines int)   { o.viewport.ScrollUp(lines) }
func (o *outputController) ScrollDown(lines int) { o.viewport.ScrollDown(lines) }
func (o *outputController) ScrollToTop()         { o.viewport.GotoTop() }
func (o *outputController) ScrollToBottom()      { o.viewport.GotoBottom() }

func (o *outputController) Title() string {
	if o.viewport.Mode() == widget.ModeLive {
		return o.Name()
	}
	if count := o.viewport.NewLineCount(); count > 0 {
		return fmt.Sprintf("%s · scroll +%d", o.Name(), count)
	}
	return o.Name() + " · scroll"
}

func (o *outputController) View(width, height int) string {
	o.setGeometry(width, height)
	return o.viewport.View()
}

func (o *outputController) setGeometry(width, height int) {
	if width > 0 {
		o.wrapWidth = width
	}
	o.hasPlacement = true
	o.viewport.SetSize(max(1, width), max(1, height))
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

func (o *outputController) appendRows(rows ...string) {
	for _, row := range rows {
		o.buffer.Append(row)
	}
	o.viewport.OnNewRows(len(rows))
}

func (o *outputController) appendMessage(text string) {
	o.appendRows(splitRows(text, o.wrapWidth)...)
}

// printServer starts or joins the short output batching window. Its return
// value tells Model whether it must schedule the first tick in a chain.
func (o *outputController) printServer(text string) (generation uint64, scheduleTick bool) {
	rows := splitRows(text, o.wrapWidth)
	if o.flushScheduled {
		o.pendingRows = append(o.pendingRows, rows...)
		return o.batchGeneration, false
	}
	o.appendRows(rows...)
	o.flushScheduled = true
	o.batchGeneration++
	return o.batchGeneration, true
}

// tick flushes one current-generation batch and reports whether output is
// still flowing. A clear invalidates already-scheduled ticks, preventing an
// old timer from closing a new batch window.
func (o *outputController) tick(generation uint64) bool {
	if generation != o.batchGeneration || !o.flushScheduled {
		return false
	}
	o.flushScheduled = false
	if len(o.pendingRows) == 0 {
		return false
	}
	o.flushPending()
	o.flushScheduled = true
	return true
}

func (o *outputController) flushPending() {
	if len(o.pendingRows) == 0 {
		return
	}
	o.appendRows(o.pendingRows...)
	o.pendingRows = nil
}

func (o *outputController) echo(text string) {
	o.flushPending()
	o.appendMessage(text)
}

func (o *outputController) setPrompt(text string) {
	text = util.ExpandTabs(text)
	if text != o.promptText {
		o.viewport.SetPrompt(text)
		o.promptText = text
	}
}

func (o *outputController) commitPrompt(text string) {
	o.flushPending()
	if text := util.ExpandTabs(text); text != "" {
		o.appendMessage(text)
	}
	o.viewport.SetPrompt("")
	o.promptText = ""
}
