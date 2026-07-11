package e2e

import (
	"testing"

	"github.com/mmcdole/rune/input"
)

// TestVerbatimSubmissionReachesWireExactly is the live-client contract for
// structured input. Bubble Tea's paste-to-composer transition is covered in
// ui/tui; this test starts at the UI submission boundary and proves that the
// real event loop, Lua core, and TCP client preserve every physical line.
func TestVerbatimSubmissionReachesWireExactly(t *testing.T) {
	c := newClient(t, "")
	c.connect()

	draft := "  say one;look  \n\n/quit\n\tlast  "
	c.ui.input <- input.Verbatim(draft)

	// Semicolons and /quit are data in verbatim mode. Each LF becomes one
	// telnet line ending; indentation, the empty line, tab, and trailing spaces
	// remain byte-exact.
	want := []byte("  say one;look  \r\n\r\n/quit\r\n\tlast  \r\n")
	c.mud.expect(want, "verbatim submission reaches the wire exactly")
}
