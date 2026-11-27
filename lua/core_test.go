package lua

import (
	"strings"
	"testing"
)

// Every embedded core file is in CoreOrder exactly once, and every listed
// file exists, so a new file cannot be silently skipped at boot.
func TestCoreOrderMatchesEmbeddedFiles(t *testing.T) {
	entries, err := coreFS.ReadDir("core")
	if err != nil {
		t.Fatal(err)
	}
	embedded := map[string]bool{}
	for _, e := range entries {
		embedded[strings.TrimSuffix(e.Name(), ".lua")] = true
	}
	seen := map[string]bool{}
	for _, name := range CoreOrder {
		if seen[name] {
			t.Errorf("%s is listed twice in CoreOrder", name)
		}
		seen[name] = true
		if !embedded[name] {
			t.Errorf("CoreOrder lists %s.lua, which is not embedded", name)
		}
	}
	for name := range embedded {
		if !seen[name] {
			t.Errorf("core/%s.lua is embedded but missing from CoreOrder", name)
		}
	}
}
