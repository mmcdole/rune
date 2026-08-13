package e2e

import "testing"

// Regression for #25: partial lines stay in the prompt overlay, and only the
// complete line reaches scrollback. Each wait makes the socket split
// deterministic.
func TestFragmentedLineAddsOnlyCompleteLineToScrollback(t *testing.T) {
	c := newClient(t, "")
	c.connect()

	partial := "E2E-I-0:"
	complete := partial + " /players/test/item#1 <-> a test item."

	c.mud.write([]byte(partial))
	c.waitFor("first partial line", func() bool {
		return c.ui.promptContains(partial)
	})

	c.mud.write([]byte(complete[len(partial):]))
	c.waitFor("updated partial line", func() bool {
		return c.ui.promptContains(complete)
	})

	// Terminate the fragmented line and follow it with a marker. Seeing the
	// marker in scrollback proves all preceding network/session events ran.
	c.mud.write([]byte("\r\nE2E-I-SYNC\r\n"))
	c.waitFor("fragmented-line sync marker", func() bool {
		return c.ui.printedContains("E2E-I-SYNC")
	})

	var partialCount, completeCount int
	printed := c.ui.printedSnapshot()
	for _, line := range printed {
		switch line {
		case partial:
			partialCount++
		case complete:
			completeCount++
		}
	}

	if partialCount != 0 || completeCount != 1 {
		t.Fatalf("fragmented line produced partial=%d complete=%d, want partial=0 complete=1; scrollback=%q",
			partialCount, completeCount, printed)
	}
}
