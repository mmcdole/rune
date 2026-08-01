package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/rune/lua"
)

// bootWithTarget builds and boots a session with a CLI connect target.
func bootWithTarget(t *testing.T, dir, target string) (*Session, *mockNetwork, *mockUI) {
	t.Helper()

	net := newMockNetwork()
	uiMock := newMockUI()
	s := New(net, uiMock, Config{
		CoreScripts:   lua.CoreScripts,
		ConfigDir:     dir,
		ConnectTarget: target,
	})
	if err := s.boot(); err != nil {
		t.Fatalf("boot failed: %v", err)
	}
	t.Cleanup(func() {
		s.timer.Stop()
	})
	return s, net, uiMock
}

// drainConnect waits for the async dial result and executes it, as
// the event loop would.
func drainConnect(t *testing.T, s *Session) {
	t.Helper()
	select {
	case cb := <-s.asyncResults:
		cb()
	case <-time.After(5 * time.Second):
		t.Fatal("connect never completed")
	}
}

// TestCLITargetConnectsOnStartup verifies "rune host port" dials on
// first boot, with the scheme-preserving address form.
func TestCLITargetConnectsOnStartup(t *testing.T) {
	s, net, uiMock := bootWithTarget(t, t.TempDir(), "mud.example.com 4000")
	drainConnect(t, s)

	if dialed := net.dialed(); len(dialed) != 1 || dialed[0] != "mud.example.com:4000" {
		t.Fatalf("dialed %v, want [mud.example.com:4000]", dialed)
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "Connected to") {
		t.Errorf("expected connected notice, got %v", printed)
	}
}

// TestCLITargetResolvesWorldNames verifies a bare world name on the
// CLI resolves through the saved-worlds store, like /connect does.
func TestCLITargetResolvesWorldNames(t *testing.T) {
	dir := t.TempDir()
	store := `{"worlds": {"arctic": {"address": "tls://mud.arcticmud.org:2701"}}}`
	if err := os.WriteFile(filepath.Join(dir, "store.json"), []byte(store), 0o644); err != nil {
		t.Fatal(err)
	}

	s, net, _ := bootWithTarget(t, dir, "arctic")
	drainConnect(t, s)

	if dialed := net.dialed(); len(dialed) != 1 || dialed[0] != "tls://mud.arcticmud.org:2701" {
		t.Fatalf("dialed %v, want the saved world address", dialed)
	}
}

// TestCLITargetNotRepeatedOnReload verifies the target is consumed by
// the first boot: /reload must not redial.
func TestCLITargetNotRepeatedOnReload(t *testing.T) {
	s, net, _ := bootWithTarget(t, t.TempDir(), "mud.example.com:4000")
	drainConnect(t, s)

	s.Reload()
	cb := <-s.asyncResults
	cb()

	if dialed := net.dialed(); len(dialed) != 1 {
		t.Fatalf("reload redialed: %v", dialed)
	}
}

// TestCLITargetUnknownWorldReportsCleanly verifies a typo'd target
// produces the /connect usage error, not a crash or a script error.
func TestCLITargetUnknownWorldReportsCleanly(t *testing.T) {
	_, net, uiMock := bootWithTarget(t, t.TempDir(), "no-such-world")

	if dialed := net.dialed(); len(dialed) != 0 {
		t.Fatalf("unexpected dial: %v", dialed)
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "[Usage]") {
		t.Errorf("expected usage guidance, got %v", printed)
	}
}
