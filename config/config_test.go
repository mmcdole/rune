package config

import "testing"

func TestResolveDirPrecedence(t *testing.T) {
	t.Setenv("RUNE_CONFIG_DIR", "/env/rune")

	if got := ResolveDir(""); got != "/env/rune" {
		t.Errorf("environment directory = %q, want /env/rune", got)
	}
	if got := ResolveDir("/flag/rune"); got != "/flag/rune" {
		t.Errorf("flag directory = %q, want /flag/rune", got)
	}
}
