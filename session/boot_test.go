package session

import (
	"os"
	"path/filepath"
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
	t.Cleanup(func() {
		s.timer.Stop()
	})
	return s, net, uiMock
}

// assertFullyBooted verifies the parts of boot that used to be
// skipped when a user script failed: the error is visible, binds
// reached the UI, and the input pipeline works end to end.
func assertFullyBooted(t *testing.T, s *Session, net *mockNetwork, uiMock *mockUI) {
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

// TestBrokenInitLuaStillBootsFully verifies the most predictable new-
// user failure - a syntax error in init.lua - costs one error report,
// not a half-dead client.
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
	assertFullyBooted(t, s, net, uiMock)
}

// TestReloadWithBrokenScriptKeepsClientAlive verifies /reload into a
// newly-broken init.lua reports the failure and leaves a working
// client, so the user can fix and /reload again.
func TestReloadWithBrokenScriptKeepsClientAlive(t *testing.T) {
	dir := t.TempDir()
	s, net, uiMock := bootSessionInDir(t, dir)
	uiMock.drainPrinted()

	// User writes a broken script, then reloads
	if err := os.WriteFile(filepath.Join(dir, "init.lua"),
		[]byte("syntax error here ((("), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Reload()
	cb := <-s.asyncResults // reload is deferred
	cb()

	printed := uiMock.drainPrinted()
	if !contains(printed, "[Script Error] init.lua") {
		t.Errorf("reload failure not reported, got %v", printed)
	}
	if !contains(printed, "Scripts reloaded") {
		t.Errorf("reload did not complete, got %v", printed)
	}
	assertFullyBooted(t, s, net, uiMock)
}
