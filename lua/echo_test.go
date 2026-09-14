package lua

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// TestEchoHookStylesAndGags verifies the "echo" event: the core
// handler styles the echo, and a user handler returning false hides
// it entirely.
func TestEchoHookStylesAndGags(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	styled, show := engine.OnEcho("north")
	if !show {
		t.Fatal("default echo unexpectedly hidden")
	}
	if !strings.Contains(styled, "> north") {
		t.Errorf("expected core styling with '> ' prefix, got %q", styled)
	}

	gag := `rune.hooks.on("echo", function(text)
		if text == "secret" then return false end
	end, {priority = 1})`
	if err := engine.DoString("gag", gag); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if _, show := engine.OnEcho("secret"); show {
		t.Error("echo hook returning false should hide the echo")
	}
	if _, show := engine.OnEcho("hello"); !show {
		t.Error("non-matching input should still echo")
	}
}

func TestEchoVisualizesTerminalControlsBeforeHooksAndFallback(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("capture", `
		rune.hooks.on("echo", function(text)
			return "captured:" .. text
		end, {priority = 1})
	`); err != nil {
		t.Fatal(err)
	}

	styled, show := engine.OnEcho("safe\x1b]52;c;x\a\tend")
	if !show {
		t.Fatal("safe echo unexpectedly hidden")
	}
	seen := text.StripANSI(styled)
	if !strings.Contains(seen, "captured:safe␛]52") || !strings.Contains(seen, "␇") || !strings.ContainsRune(seen, '\t') {
		t.Fatalf("styled echo did not visualize controls: %q", seen)
	}

	if err := engine.DoString("break-hooks", `rune.hooks = nil`); err != nil {
		t.Fatal(err)
	}
	styled, show = engine.OnEcho("\x1b[2J\x00")
	if !show || !strings.Contains(text.StripANSI(styled), "␛[2J␀") {
		t.Fatalf("degraded echo is unsafe: %q (show=%v)", styled, show)
	}
}
