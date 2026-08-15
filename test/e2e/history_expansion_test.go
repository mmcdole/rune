package e2e

import (
	"bytes"
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

// Regression: repeat expansion used to search history after the active raw
// submission had been inserted. In a compound submission, the `!` component
// therefore selected `north;!` itself and recursively sent north until Lua
// exhausted its stack.
func TestCompoundHistoryExpansionUsesPriorSnapshot(t *testing.T) {
	c := newClient(t, "")
	c.connect()

	const previous = "E2E-repeat-previous"
	c.ui.events <- ui.InputSubmittedMsg{Submission: input.Command(previous)}
	c.mud.expect([]byte(previous+"\r\n"), "initial command")

	c.ui.events <- ui.InputSubmittedMsg{Submission: input.Command("north;!")}
	c.ui.events <- ui.InputSubmittedMsg{Submission: input.Command("E2E-repeat-done")}
	c.mud.expect([]byte("E2E-repeat-done\r\n"), "event loop after compound repeat")

	if got := bytes.Count(c.mud.got, []byte("north\r\n")); got != 1 {
		t.Fatalf("compound repeat sent north %d times, want exactly once; wire=%q", got, c.mud.got)
	}
	if got := bytes.Count(c.mud.got, []byte(previous+"\r\n")); got != 2 {
		t.Fatalf("previous command appeared %d times, want initial send plus one repeat; wire=%q", got, c.mud.got)
	}
}
