package paths

import "testing"

func TestConfigDirPrecedence(t *testing.T) {
	t.Setenv("RUNE_CONFIG_DIR", "/env/rune")

	if got := ConfigDir(""); got != "/env/rune" {
		t.Errorf("environment directory = %q, want /env/rune", got)
	}
	if got := ConfigDir("/flag/rune"); got != "/flag/rune" {
		t.Errorf("flag directory = %q, want /flag/rune", got)
	}
}
