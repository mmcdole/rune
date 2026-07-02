package main

import (
	"testing"
)

func TestClassifyArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		scripts []string
		target  string
		wantErr bool
	}{
		{"empty", nil, nil, "", false},
		{"host port", []string{"mud.example.com", "4000"}, nil, "mud.example.com 4000", false},
		{"host port tls", []string{"mud.example.com", "4000", "tls"}, nil, "mud.example.com 4000 tls", false},
		{"host:port", []string{"mud.example.com:4000"}, nil, "mud.example.com:4000", false},
		{"scheme address", []string{"tls://mud.example.com:4000"}, nil, "tls://mud.example.com:4000", false},
		{"world name", []string{"arctic"}, nil, "arctic", false},
		{"script only", []string{"combat.lua"}, []string{"combat.lua"}, "", false},
		{"script by path", []string{"scripts/combat"}, []string{"scripts/combat"}, "", false},
		{"scripts and target", []string{"combat.lua", "arctic", "ui.lua"},
			[]string{"combat.lua", "ui.lua"}, "arctic", false},
		{"too many words", []string{"a", "b", "c", "d"}, nil, "", true},
	}
	for _, c := range cases {
		scripts, target, err := classifyArgs(c.args)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", c.name)
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
		if len(scripts) != len(c.scripts) {
			t.Errorf("%s: scripts = %v, want %v", c.name, scripts, c.scripts)
			continue
		}
		for i := range scripts {
			if scripts[i] != c.scripts[i] {
				t.Errorf("%s: scripts = %v, want %v", c.name, scripts, c.scripts)
				break
			}
		}
	}
}
