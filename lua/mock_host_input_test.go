package lua

import (
	"testing"

	"github.com/mmcdole/rune/input"
)

func TestMockSetInputUsesVerbatimAdmissionAndStickyReset(t *testing.T) {
	host := NewMockHost()

	for _, value := range []string{"one\ntwo", "safe\x1b[31m", "left\u202eright"} {
		host.SetInputSubmission(input.Command("seed"))
		host.SetInput(value)
		if host.InputMode != input.ModeVerbatim {
			t.Fatalf("SetInput(%q) mode = %s, want verbatim", value, host.InputMode)
		}
	}

	host.SetInput("plain; /quit")
	if got := host.GetInput(); got != "plain; /quit" {
		t.Fatalf("sticky input = %q, want %q", got, "plain; /quit")
	}
	if host.InputMode != input.ModeVerbatim {
		t.Fatalf("plain replacement mode = %s, want sticky verbatim", host.InputMode)
	}

	host.SetInput("")
	if got := host.GetInput(); got != "" {
		t.Fatalf("cleared input = %q, want empty", got)
	}
	if host.InputMode != input.ModeCommand {
		t.Fatalf("cleared input mode = %s, want command", host.InputMode)
	}
}

func TestMockHostInputCursorUsesByteBoundaries(t *testing.T) {
	host := &MockHost{InputText: "café"}

	host.InputSetCursor(4)
	if got, want := host.InputGetCursor(), 3; got != want {
		t.Fatalf("cursor inside UTF-8 sequence = %d, want %d", got, want)
	}

	host.InputSetCursor(6)
	if got, want := host.InputGetCursor(), len("café"); got != want {
		t.Fatalf("cursor beyond input = %d, want %d", got, want)
	}

	host.InputSetCursor(-1)
	if got := host.InputGetCursor(); got != 0 {
		t.Fatalf("negative cursor = %d, want 0", got)
	}
}
