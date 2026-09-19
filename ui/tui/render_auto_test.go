package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/mmcdole/rune/ui"
	"strings"
	"testing"
)

func autoLayoutFixture(kind string, draft int) *Model {
	m := renderFixture(270, 66, false)
	output := ui.LayoutNode{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone}
	input := ui.LayoutNode{Type: ui.LayoutTypeInput, Size: ui.AutoSize()}
	chat := ui.LayoutNode{Type: ui.LayoutTypePane, Name: "chat", Size: ui.AutoSize()}
	if kind == "auto_side" {
		m.layout.Root = ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{chat, output, input}}
	} else if kind == "input_beside_pane" {
		chat.Size = ui.Cells(90)
		input.Size = ui.Fraction(1)
		m.layout.Root = ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{output,
			{Type: ui.LayoutTypeRow, Size: ui.AutoSize(), Children: []ui.LayoutNode{input, chat}},
		}}
	}
	if _, err := ui.NormalizeLayoutTree(m.layout); err != nil {
		panic(err)
	}
	m.pane("chat").Write("chat line")
	if draft > 0 {
		m.input.SetValue(strings.Repeat("say This is an unchanged draft line.\n", draft))
	}
	m.applyLayout()
	m.render()
	m.renderInterval, m.throttled = defaultRenderInterval, true
	return m
}

func BenchmarkRenderAutoLayoutOutput(b *testing.B) {
	for _, kind := range []string{"default", "auto_side", "input_beside_pane"} {
		for _, draft := range []int{0, 1000} {
			b.Run(fmt.Sprintf("%s/draft=%d", kind, draft), func(b *testing.B) {
				m := autoLayoutFixture(kind, draft)
				b.ReportAllocs()
				for b.Loop() {
					m.Update(ui.PrintLineMsg("incoming"))
					m.View()
				}
			})
		}
	}
}

func TestLayoutChangeErasesUncoveredCells(t *testing.T) {
	m := renderFixture(80, 24, false)
	output := ui.LayoutNode{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Border: ui.PaneBorderNone, Size: ui.Cells(3)}
	chat := ui.LayoutNode{Type: ui.LayoutTypePane, Name: "chat", Border: ui.PaneBorderNone, Size: ui.Cells(10)}
	input := ui.LayoutNode{Type: ui.LayoutTypeInput, Size: ui.Cells(3)}
	m.layout.Root = ui.LayoutNode{Type: ui.LayoutTypeColumn, Children: []ui.LayoutNode{output, chat, input}}
	m.pane("chat").Write(strings.Repeat("GAP-MARKER\n", 8))
	m.applyLayout()
	if !strings.Contains(m.View().Content, "GAP-MARKER") {
		t.Fatal("setup missing marker")
	}
	m.layout.Root.Children = []ui.LayoutNode{output, input}
	m.applyLayout()
	if strings.Contains(m.View().Content, "GAP-MARKER") {
		t.Fatal("removed pane remains in uncovered cells")
	}
}

func BenchmarkRenderDraftCursor(b *testing.B) {
	for _, kind := range []string{"default", "auto_side", "input_beside_pane"} {
		b.Run(kind, func(b *testing.B) {
			m := autoLayoutFixture(kind, 1000)
			b.ReportAllocs()
			for b.Loop() {
				m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
				m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
				m.View()
			}
		})
	}
}
