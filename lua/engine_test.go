package lua

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/rune/version"
)

// TestDoFileLoadsSiblingModuleSurvivesClobberedPackageAndRestoresPath verifies
// DoFile's script-loading invariants: a file can require a module beside it, a
// script that clobbers the package global cannot panic the process on the next
// file load, and a successful load leaves package.path exactly as it found it.
func TestDoFileLoadsSiblingModuleSurvivesClobberedPackageAndRestoresPath(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	dir := t.TempDir()
	module := filepath.Join(dir, "sibling.lua")
	if err := os.WriteFile(module, []byte(`return "file ran"`), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "loaded.lua")
	if err := os.WriteFile(script, []byte(`rune.send_raw(require("sibling"))`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Normal path: package.path is restored byte-identically.
	if err := engine.DoString("snap", `path_before = package.path`); err != nil {
		t.Fatal(err)
	}
	if err := engine.DoFile(script); err != nil {
		t.Fatalf("DoFile: %v", err)
	}
	if err := engine.DoString("check", `assert(package.path == path_before, "package.path not restored")`); err != nil {
		t.Error(err)
	}

	// Clobbered package global: the load still runs instead of panicking.
	if err := engine.DoString("sabotage", `package = 5`); err != nil {
		t.Fatal(err)
	}
	if err := engine.DoFile(script); err != nil {
		t.Fatalf("DoFile with clobbered package: %v", err)
	}
	if sent := host.DrainNetworkCalls(); len(sent) != 2 {
		t.Errorf("expected the file to run both times, sent %v", sent)
	}
}

// TestClobberedStateTableDoesNotPanic verifies a user script that
// overwrites rune._state cannot crash the client: UpdateState skips the
// push instead of panicking the process on an unchecked type assertion.
// State pushes ride connection, resize, and scroll events, so before
// the checked lookup this was a delayed hard crash from one bad script
// line.
func TestClobberedStateTableDoesNotPanic(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	for _, sabotage := range []string{
		"rune._state = 5",
		`rune._state = "text"`,
		"rune._state = nil",
	} {
		if err := engine.DoString("sabotage", sabotage); err != nil {
			t.Fatalf("%s: %v", sabotage, err)
		}
		engine.UpdateState(ClientState{Connected: true, Address: "mud.example.com:4000", Width: 80, Height: 24})
	}

	// The VM survives and everything else keeps working.
	if err := engine.DoString("after", `rune.send_raw("still alive")`); err != nil {
		t.Fatalf("VM unusable after clobbered-state update: %v", err)
	}
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "still alive" {
		t.Errorf("expected send after clobbered-state update, got %v", sent)
	}

	// Re-init (what /reload does) rebuilds the mirror and pushes land again.
	if err := engine.Init(); err != nil {
		t.Fatal(err)
	}
	engine.UpdateState(ClientState{Connected: true, Address: "mud.example.com:4000"})
	if err := engine.DoString("check", `
		assert(rune._state.connected == true, "state push lost after reload")
		assert(rune._state.address == "mud.example.com:4000")
	`); err != nil {
		t.Error(err)
	}
}

// TestVersionSingleSourced verifies rune.version comes from the Go
// version package (the same value TTYPE/MNES report).
func TestVersionSingleSourced(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("check",
		`assert(rune.version == "`+version.Number+`", "rune.version = " .. tostring(rune.version))`); err != nil {
		t.Error(err)
	}
}

// TestRegistryGrowsForLargeConcat verifies the VM can serialize large
// tables. This was a gopher-lua failure mode: table.concat pushed every
// element onto a fixed-size data stack, so tables past a few thousand
// entries (e.g. CBOR-encoding a mob database) failed even though
// building or decoding the same table worked. The test stays as a
// regression guard on the backend seam.
func TestRegistryGrowsForLargeConcat(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("bigconcat", `
		local t = {}
		for i = 1, 20000 do t[i] = "chunk_" .. i end
		local blob = table.concat(t)
		assert(#blob > 0)
	`)
	if err != nil {
		t.Fatalf("large table.concat failed: %v", err)
	}
}

// TestRaisedErrorsCarrySinglePosition verifies host-raised errors
// (argument type errors, c.Errorf) carry exactly one script position
// prefix on the active backend — a doubled prefix means the backend
// stacked its own decoration on top of the seam's Where().
func TestRaisedErrorsCarrySinglePosition(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("prefix.lua", "rune._send_raw({})")
	if err == nil {
		t.Fatal("expected an argument type error")
	}
	first := strings.SplitN(err.Error(), "\n", 2)[0]
	if !strings.Contains(first, "string expected, got table") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(first, ":1: "); got != 1 {
		t.Errorf("want exactly one position prefix, got %d in %q", got, first)
	}
}
