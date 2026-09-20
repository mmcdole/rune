package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mmcdole/rune/ui"
)

func BenchmarkRenderBarUpdate(b *testing.B) {
	for _, kind := range []string{"default", "auto_side", "input_beside_pane"} {
		b.Run(kind, func(b *testing.B) {
			m := autoLayoutFixture(kind, 1000)
			m.layout.Root.Children = append([]ui.LayoutNode{
				{Type: ui.LayoutTypeBar, Name: "status", Size: ui.AutoSize()},
			}, m.layout.Root.Children...)
			updates := []ui.UpdateBarsMsg{{"status": {Left: "HP:100"}}, {"status": {Left: "HP:99"}}}
			m.Update(updates[0])
			m.applyLayout()
			n := 0
			b.ReportAllocs()
			for b.Loop() {
				m.Update(updates[n%2])
				n++
			}
		})
	}
}

func BenchmarkRenderDraftEdit(b *testing.B) {
	for _, kind := range []string{"default", "auto_side", "input_beside_pane"} {
		b.Run(kind, func(b *testing.B) {
			m := autoLayoutFixture(kind, 1000)
			events := make(chan ui.UIEvent, 16)
			m.events = events
			b.ReportAllocs()
			for b.Loop() {
				m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
				m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
				for len(events) > 0 {
					<-events
				}
			}
		})
	}
}
