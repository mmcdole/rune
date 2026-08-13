package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/rune/lua"
)

// bootSessionInDir builds and boots a session against a caller-owned
// config dir without failing the test on boot errors - these tests
// exist precisely to exercise boot with broken user scripts.
func bootSessionInDir(t *testing.T, dir string) (*Session, *mockNetwork, *mockUI) {
	t.Helper()

	net := newMockNetwork()
	uiMock := newMockUI()
	s := New(net, uiMock, Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   dir,
	})
	if err := s.boot(); err != nil {
		t.Fatalf("boot must not fail on user-script errors: %v", err)
	}
	cleanupTestSession(t, s)
	return s, net, uiMock
}

// TestBootDoesNotReportLuaReentryErrors exercises the complete startup path.
// Core script configuration callbacks synchronously query binds and bars, so
// boot must reuse the callback's active Lua execution rather than reentering
// the already-running State.
func TestBootDoesNotReportLuaReentryErrors(t *testing.T) {
	_, _, uiMock := bootSessionInDir(t, t.TempDir())

	for _, printed := range uiMock.drainPrinted() {
		if strings.Contains(printed, "state is executing") {
			t.Fatalf("boot reported Lua reentry error: %q", printed)
		}
	}
}

// assertSessionUsable checks that core policy and presentation state were
// installed even when a user script failed.
func assertSessionUsable(t *testing.T, s *Session, net *mockNetwork, uiMock *mockUI) {
	t.Helper()

	if binds := uiMock.pushedBinds(); len(binds) == 0 {
		t.Error("binds never pushed to the UI - key routing is broken")
	}

	net.connected = true
	userInput(s, "north")
	if sent := net.drainSent(); len(sent) != 1 || sent[0] != "north" {
		t.Errorf("input pipeline broken after script error: sent %v", sent)
	}

	userInput(s, "/help")
	if printed := uiMock.drainPrinted(); !contains(printed, "/connect") {
		t.Error("slash commands broken after script error")
	}
}

// TestBrokenInitLuaStillBootsFully verifies that a syntax error in init.lua is
// reported without preventing core policy from loading.
func TestBrokenInitLuaStillBootsFully(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.lua"),
		[]byte("this is not lua ((("), 0o644); err != nil {
		t.Fatal(err)
	}

	s, net, uiMock := bootSessionInDir(t, dir)

	if printed := uiMock.drainPrinted(); !contains(printed, "[Script Error] init.lua") {
		t.Errorf("script failure not reported, got %v", printed)
	}
	assertSessionUsable(t, s, net, uiMock)
}

// TestReloadWithBrokenScriptKeepsClientAlive verifies that /reload reports an
// init.lua error while leaving the rebuilt Session usable.
func TestReloadWithBrokenScriptKeepsClientAlive(t *testing.T) {
	dir := t.TempDir()
	s, net, uiMock := bootSessionInDir(t, dir)
	uiMock.drainPrinted()

	if err := os.WriteFile(filepath.Join(dir, "init.lua"),
		[]byte("syntax error here ((("), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Reload()
	awaitInternalEvent(t, s)

	printed := uiMock.drainPrinted()
	if !contains(printed, "[Script Error] init.lua") {
		t.Errorf("reload failure not reported, got %v", printed)
	}
	if !contains(printed, "Scripts reloaded") {
		t.Errorf("reload did not complete, got %v", printed)
	}
	assertSessionUsable(t, s, net, uiMock)
}
