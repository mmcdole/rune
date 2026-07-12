package e2e

import "testing"

// TestFragmentedLineDoesNotCommitPromptSnapshots reproduces issue #25: the
// VikingMUD inventory corruption reported by Moreldir on 2026-07-12. Viking's
// wizard
// I command builds one logical row with several write() calls. When those
// writes arrive in separate socket reads, Rune exposes the growing tail as
// prompt snapshots before the terminating CRLF arrives.
//
// Each wait is a causal barrier: the next fragment is not written until the
// live client has displayed the preceding snapshot. This makes the read
// fragmentation deterministic without timing assumptions.
func TestFragmentedLineDoesNotCommitPromptSnapshots(t *testing.T) {
	c := newClient(t, "")
	c.connect()

	partial := "E2E-I-0:"
	complete := partial + " /players/test/item#1 <-> a test item."

	c.mud.write([]byte(partial))
	c.waitFor("first fragmented-line prompt snapshot", func() bool {
		return c.ui.promptContains(partial)
	})

	c.mud.write([]byte(complete[len(partial):]))
	c.waitFor("completed fragmented-line prompt snapshot", func() bool {
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
