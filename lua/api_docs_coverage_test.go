package lua

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The docs site (website/src/content/docs/reference/api/) is the canonical
// API reference. This test extracts every public rune.* function defined in
// the embedded Lua core and asserts it is mentioned somewhere in those
// reference pages, so a new public function cannot ship undocumented.

// Internal surfaces, deliberately undocumented. Documenting one of these
// would turn an implementation detail into a compatibility promise.
var undocumentedInternal = map[string]bool{
	"rune.guarded_call":        true,
	"rune.caller_source":       true,
	"rune.substitute_captures": true,
	"rune.registry.new":        true,
	"rune.command.dispatch":    true,
	"rune.alias.process":       true,
	"rune.trigger.process":     true,
	"rune.hooks.call":          true,
}

var undocumentedPrefixes = []string{
	"rune.completion.",
}

// The per-registry management suite is documented once, as the shared
// registry contract on reference/api/index.md (#managing), rather than
// enumerated on every namespace page.
var sharedContractSuffixes = map[string]bool{
	"enable":       true,
	"disable":      true,
	"remove":       true,
	"cancel":       true,
	"list":         true,
	"count":        true,
	"clear":        true,
	"remove_group": true,
}

func isInternalName(name string) bool {
	if undocumentedInternal[name] {
		return true
	}
	for _, p := range undocumentedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, seg := range strings.Split(name, ".") {
		if strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return false
}

func publicCoreFunctions(t *testing.T) []string {
	t.Helper()
	fnRe := regexp.MustCompile(`(?m)^function (rune\.[A-Za-z0-9_.]+)\s*\(`)
	entries, err := os.ReadDir("core")
	if err != nil {
		t.Fatalf("reading core/: %v", err)
	}
	var names []string
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		src, err := os.ReadFile(filepath.Join("core", e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, m := range fnRe.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			if isInternalName(name) || seen[name] {
				continue
			}
			last := name[strings.LastIndex(name, ".")+1:]
			if sharedContractSuffixes[last] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) < 30 {
		t.Fatalf("only %d public functions extracted from core/*.lua; extraction is likely broken", len(names))
	}
	return names
}

// documentedNames collects every rune.* token from the reference pages,
// expanding slash-compressed mentions ("rune.trigger.enable/disable/remove")
// into their individual names.
func documentedNames(t *testing.T) map[string]bool {
	t.Helper()
	docDir := filepath.Join("..", "website", "src", "content", "docs", "reference", "api")
	entries, err := os.ReadDir(docDir)
	if err != nil {
		t.Fatalf("reading %s: %v", docDir, err)
	}
	tokenRe := regexp.MustCompile(`rune\.[A-Za-z0-9_.]+(?:/[A-Za-z0-9_]+)*`)
	documented := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".mdx")) {
			continue
		}
		page, err := os.ReadFile(filepath.Join(docDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, tok := range tokenRe.FindAllString(string(page), -1) {
			parts := strings.Split(tok, "/")
			base := strings.TrimRight(parts[0], ".")
			documented[base] = true
			dot := strings.LastIndex(base, ".")
			if dot < 0 {
				continue
			}
			prefix := base[:dot+1]
			for _, alt := range parts[1:] {
				documented[prefix+alt] = true
			}
		}
	}
	if len(documented) == 0 {
		t.Fatalf("no rune.* tokens found under %s; are the reference pages present?", docDir)
	}
	return documented
}

func TestPublicAPIDocumented(t *testing.T) {
	documented := documentedNames(t)
	for _, name := range publicCoreFunctions(t) {
		if !documented[name] {
			t.Errorf("public function %s (defined in lua/core) is not mentioned in website/src/content/docs/reference/api/", name)
		}
	}
}
