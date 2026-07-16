package lua

import "testing"

func TestClipboardSetReachesHost(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("test", `rune.clipboard.set("note contents")`); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if len(host.ClipboardCalls) != 1 || host.ClipboardCalls[0] != "note contents" {
		t.Errorf("got clipboard calls %q, want [\"note contents\"]", host.ClipboardCalls)
	}
}
