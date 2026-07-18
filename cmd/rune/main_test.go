package main

import (
	"strings"
	"testing"
)

func TestConnectTarget(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		target      string
		errContains string
	}{
		{"empty", nil, "", ""},
		{"host port", []string{"mud.example.com", "4000"}, "mud.example.com 4000", ""},
		{"host port tls", []string{"mud.example.com", "4000", "tls"}, "mud.example.com 4000 tls", ""},
		{"host:port", []string{"mud.example.com:4000"}, "mud.example.com:4000", ""},
		{"scheme address", []string{"tls://mud.example.com:4000"}, "tls://mud.example.com:4000", ""},
		{"world name", []string{"arctic"}, "arctic", ""},
		{"too many words", []string{"a", "b", "c", "d"}, "", "too many arguments"},
	}
	for _, c := range cases {
		target, err := connectTarget(c.args)
		if c.errContains != "" {
			if err == nil {
				t.Errorf("%s: expected error", c.name)
			} else if !strings.Contains(err.Error(), c.errContains) {
				t.Errorf("%s: error = %q, want it to contain %q", c.name, err, c.errContains)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if target != c.target {
			t.Errorf("%s: target = %q, want %q", c.name, target, c.target)
		}
	}
}

func TestUsageTextNamesWorldAndAddress(t *testing.T) {
	usage := usageText("/tmp/rune-config")
	if !strings.Contains(usage, "rune [--config-dir <dir>] [world | address]") {
		t.Errorf("usage synopsis does not name world and address:\n%s", usage)
	}
	if !strings.Contains(usage, "--config-dir <dir>") ||
		!strings.Contains(usage, "(default: /tmp/rune-config)") {
		t.Errorf("usage does not show config-dir and its effective default:\n%s", usage)
	}
	for _, form := range []string{"host:port", "host port", "tls://host:port"} {
		if !strings.Contains(usage, form) {
			t.Errorf("usage does not explain address form %q:\n%s", form, usage)
		}
	}
}
