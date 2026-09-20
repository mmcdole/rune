package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/ui"
)

// Fixtures have fixed history, dimensions, and content; setup is never timed.
// No benchmark waits for a wall-clock frame timer. Explicit ticks make the
// amount of work per operation independent of the machine's speed.
func renderFixture(width, height int, sidebar bool) *Model {
	m := NewModel(make(chan ui.UIEvent, 4096))
	m.width, m.height, m.initialized = width, height, true
	if sidebar {
		m.layout = ui.LayoutTree{Root: ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{
			{Type: ui.LayoutTypeRow, Dividers: true, Children: []ui.LayoutNode{
				{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone},
				{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Cells(width / 3)},
			}},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		}}}
	}
	m.applyLayout()
	for n := range 200 {
		line := fmt.Sprintf("\x1b[32mRoom %03d\x1b[0m: a \x1b[1;31mdragon\x1b[0m watches the northern gate.", n)
		m.output.Write(line)
		if sidebar {
			m.pane("chat").Write(line)
		}
	}
	m.output.SetPrompt("HP:100 >")
	m.applyLayout()
	m.render()
	return m
}

func BenchmarkRenderScreen(b *testing.B) {
	for _, size := range [][2]int{{80, 24}, {270, 66}} {
		b.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(b *testing.B) {
			m := renderFixture(size[0], size[1], true)
			b.ReportAllocs()
			for b.Loop() {
				m.render()
			}
		})
	}
}

func BenchmarkRenderLayout(b *testing.B) {
	for _, sidebar := range []bool{false, true} {
		b.Run(fmt.Sprintf("sidebar=%t", sidebar), func(b *testing.B) {
			m := renderFixture(270, 66, sidebar)
			b.ReportAllocs()
			for b.Loop() {
				m.applyLayout()
			}
		})
	}
}

func BenchmarkRenderNestedLayout(b *testing.B) {
	for _, size := range [][2]int{{270, 66}, {20, 3}} {
		b.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(b *testing.B) {
			m := renderFixture(size[0], size[1], false)
			m.layout.Root = staggeredLayout()
			for _, row := range []*ui.LayoutNode{&m.layout.Root.Children[0], &m.layout.Root.Children[2]} {
				for i := range row.Children {
					if row.Children[i].Type == ui.LayoutTypePane {
						row.Children[i].Border = ui.PaneBorderFull
					}
				}
			}
			root := m.resolveNode(m.layout.Root, m.width, axisVertical)
			if root == nil {
				b.Fatal("nested fixture did not resolve")
			}
			assignSharedEdges(root, 0)
			if constrained := m.allocateChildren(root, m.height, axisVertical, m.width).constrained; constrained != (m.height == 3) {
				b.Fatalf("unexpected fallback: %t", constrained)
			}
			b.ReportAllocs()
			for b.Loop() {
				m.applyLayout()
			}
		})
	}
}

// One incoming line inside an already-open throttle window. This exposes
// per-message work that the render throttle does not eliminate.
func BenchmarkRenderUpdate(b *testing.B) {
	for _, lines := range []int{0, 100, 1000} {
		b.Run(fmt.Sprintf("draft_lines=%d", lines), func(b *testing.B) {
			m := renderFixture(270, 66, false)
			if lines > 0 {
				m.input.SetValue(strings.Repeat("say This is a representative line of pasted MUD commands.\n", lines))
			}
			m.applyLayout()
			m.renderInterval, m.throttled = defaultRenderInterval, true
			b.ReportAllocs()
			for b.Loop() {
				m.Update(ui.PrintLineMsg("a line of MUD output"))
				m.View()
			}
		})
	}
}

// Each operation is exactly 100 appends, ten prompt updates, and one frame
// boundary. Unlike a lines/second test, faster machines do not render more often.
func BenchmarkRenderFlood(b *testing.B) {
	m := renderFixture(270, 66, true)
	m.renderInterval, m.throttled = defaultRenderInterval, true
	n := 0
	b.ReportAllocs()
	for b.Loop() {
		for line := range 100 {
			m.Update(ui.PrintLineMsg(fmt.Sprintf("\x1b[32mIncoming %d\x1b[0m: a line of MUD output", n)))
			n++
			m.View()
			if line%10 == 9 {
				m.Update(ui.SetPromptMsg("HP:100 >"))
				m.View()
			}
		}
		m.Update(renderTick{})
		m.View()
	}
	b.ReportMetric(100, "lines/op")
}

type renderByteCounter struct{ bytes int64 }

func (w *renderByteCounter) Write(p []byte) (int, error) {
	w.bytes += int64(len(p))
	return len(p), nil
}

// Includes Rune Update/View, the ANSI->cells conversion used by Bubble Tea,
// and Ultraviolet's terminal diff/encoding. The writer counts bytes rather
// than measuring an OS pipe or a particular terminal emulator. Bubble Tea's
// scheduling, terminal negotiation, and physical display latency are excluded.
func BenchmarkRenderPipeline(b *testing.B) {
	for _, change := range []string{"prompt", "scroll"} {
		b.Run(change, func(b *testing.B) {
			m := renderFixture(270, 66, true)
			canvas := newCanvas(m.width, m.height)
			var output renderByteCounter
			renderer := uv.NewTerminalRenderer(&output, []string{"TERM=xterm-256color"})
			renderer.SetColorProfile(colorprofile.TrueColor)
			renderer.SetWidthMethod(ansi.GraphemeWidth)
			renderer.SetScrollOptim(true)
			flush := func() {
				canvas.Clear()
				uv.NewStyledString(m.View().Content).Draw(canvas, canvas.Bounds())
				renderer.Render(canvas.RenderBuffer)
				if err := renderer.Flush(); err != nil {
					b.Fatal(err)
				}
			}
			flush() // exclude the initial full-screen write
			output.bytes = 0
			n := 1
			b.ReportAllocs()
			for b.Loop() {
				if change == "prompt" {
					m.Update(ui.SetPromptMsg(fmt.Sprintf("HP:%d >", 100-n%2)))
				} else {
					m.Update(ui.PrintLineMsg(fmt.Sprintf("\x1b[32mIncoming %d\x1b[0m: new output", n)))
				}
				flush()
				n++
			}
			b.ReportMetric(float64(output.bytes)/float64(b.N), "terminal-B/op")
		})
	}
}

// These messages do not change the screen. Measure the cost of ignoring them.
func BenchmarkRenderUnchanged(b *testing.B) {
	m := renderFixture(270, 66, true)
	m.renderInterval = defaultRenderInterval
	b.ReportAllocs()
	for b.Loop() {
		m.Update(ui.SetPromptMsg("HP:100 >"))
		m.View()
		m.Update(renderTick{})
		m.View()
	}
}

func BenchmarkRenderResize(b *testing.B) {
	m := renderFixture(80, 24, true)
	events := make(chan ui.UIEvent, 8)
	m.events = events
	n := 0
	b.ReportAllocs()
	for b.Loop() {
		size := [][2]int{{80, 24}, {120, 40}}[n%2]
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.View()
		// Drain the resize event synchronously, so a long benchmark cannot
		// fill the Session queue and start rendering overflow warnings.
		for len(events) > 0 {
			<-events
		}
		n++
	}
}

// Border discovery visits provisional and final widths during layout. Draft
// label formatting is deliberately outside the work this query needs.
func BenchmarkRenderInputBorders(b *testing.B) {
	for _, width := range []int{180, 270} {
		b.Run(fmt.Sprintf("width=%d", width), func(b *testing.B) {
			m := autoLayoutFixture("input_beside_pane", 1000)
			b.ReportAllocs()
			for b.Loop() {
				m.inputBorders(width, 10)
			}
		})
	}
}
