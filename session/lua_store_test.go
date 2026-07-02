package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drake/rune/lua"
)

// newTestSessionInDir is newTestSession with a caller-owned config
// dir, so tests can boot a second "client run" against the same disk
// state.
func newTestSessionInDir(t *testing.T, dir string) (*Session, *mockNetwork, *mockUI) {
	t.Helper()

	net := newMockNetwork()
	uiMock := newMockUI()
	s := New(net, uiMock, Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   dir,
	})
	if err := s.boot(); err != nil {
		t.Fatalf("boot failed: %v", err)
	}
	uiMock.drainPrinted()
	t.Cleanup(func() {
		s.timer.Stop()
	})
	return s, net, uiMock
}

// TestStoreSurvivesRestart writes through rune.store in one session
// and reads it back in a fresh session sharing the config dir - the
// disk round trip that rune.session state deliberately lacks.
func TestStoreSurvivesRestart(t *testing.T) {
	dir := t.TempDir()

	s1, _, _ := newTestSessionInDir(t, dir)
	if err := s1.engine.DoString("write",
		`assert(rune.store.set("worlds", { arctic = { address = "mud.arcticmud.org:2700" } }))`); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "store.json")); err != nil {
		t.Fatalf("store.json not written: %v", err)
	}

	s2, _, _ := newTestSessionInDir(t, dir)
	if err := s2.engine.DoString("read", `
		local worlds = rune.store.get("worlds")
		assert(worlds and worlds.arctic, "worlds table missing after restart")
		assert(worlds.arctic.address == "mud.arcticmud.org:2700")
	`); err != nil {
		t.Fatalf("store did not survive restart: %v", err)
	}
}

// TestCorruptStoreBackedUpNotDiscarded verifies a corrupt store.json
// is preserved as .bak, reported at boot, and the store stays usable.
func TestCorruptStoreBackedUpNotDiscarded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	net := newMockNetwork()
	uiMock := newMockUI()
	s := New(net, uiMock, Config{CoreScripts: lua.CoreScripts, ConfigDir: dir})
	if err := s.boot(); err != nil {
		t.Fatalf("boot failed: %v", err)
	}
	t.Cleanup(func() { s.timer.Stop() })

	if !contains(uiMock.drainPrinted(), "corrupt") {
		t.Error("corrupt store not reported at boot")
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("corrupt store.json not preserved as .bak: %v", err)
	}
	if !strings.Contains(string(backup), "{not json") {
		t.Errorf("backup content mangled: %q", backup)
	}

	// Store still works after recovery
	if err := s.engine.DoString("write", `assert(rune.store.set("k", "v"))`); err != nil {
		t.Fatalf("store unusable after corrupt-file recovery: %v", err)
	}
}
