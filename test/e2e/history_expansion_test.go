package e2e

import (
	"bytes"
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

// History expansion runs before the submitted line enters history, so every
// expansion in a compound line searches only earlier commands. Otherwise the
// `!` in `north;!` could select that same line and recurse.
func TestCompoundHistoryExpansionSearchesOnlyEarlierHistory(t *testing.T) {
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
