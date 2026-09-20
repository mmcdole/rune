package widget

import (
	"fmt"
	"testing"

	"github.com/mmcdole/rune/ui/tui/style"
)

func BenchmarkRenderSearch(b *testing.B) {
	for _, colored := range []bool{false, true} {
		for _, query := range []string{"room", "not-present"} {
			b.Run(fmt.Sprintf("colored=%t/query=%s", colored, query), func(b *testing.B) {
				buffer := NewScrollback(100000)
				line := "You see an ordinary room with exits north and south."
				if colored {
					line = "\x1b[32m" + line + "\x1b[0m"
				}
				for range 100000 {
					buffer.Append(line)
				}
				search := NewSearch(buffer, style.DefaultStyles())
				search.Open(query, SearchScope{})
				b.ReportAllocs()
				for b.Loop() {
					search.Reopen(query)
				}
			})
		}
	}
}

func BenchmarkRenderPane(b *testing.B) {
	for _, height := range []int{24, 66, 200} {
		b.Run(fmt.Sprintf("rows=%d", height), func(b *testing.B) {
			pane := NewPane("chat")
			for range 500 {
				pane.Write("\x1b[32mSomeone says:\x1b[0m a short chat message.")
			}
			pane.SetSize(80, height)
			b.ReportAllocs()
			for b.Loop() {
				pane.View()
			}
		})
	}
}
