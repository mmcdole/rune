package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLogCapturesSessionToFile drives a logged session end-to-end and
// verifies the file reads like the screen: ANSI-stripped output, the
// local echo of typed input, no gagged lines, start/stop stamps.
func TestLogCapturesSessionToFile(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	path := filepath.Join(s.config.ConfigDir, "session.log")
	userInput(s, "/log start "+path)
	if _, active := s.LogStatus(); !active {
		t.Fatal("log not active after /log start")
	}

	if err := s.engine.DoString("gag",
		`rune.trigger.contains("secret", function() end, {gag=true})`); err != nil {
		t.Fatal(err)
	}

	serverLine(s, "Hello \x1b[31mred\x1b[0m world")
	serverLine(s, "a secret line")
	userInput(s, "kill rat")
	userInput(s, "/log stop")

	if _, active := s.LogStatus(); active {
		t.Fatal("log still active after /log stop")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "Hello red world") {
		t.Errorf("expected ANSI-stripped output line in log, got:\n%s", content)
	}
	if strings.Contains(content, "\x1b") {
		t.Errorf("log contains raw ANSI escapes:\n%q", content)
	}
	if !strings.Contains(content, "> kill rat") {
		t.Errorf("expected echoed input in log, got:\n%s", content)
	}
	if strings.Contains(content, "secret") {
		t.Errorf("gagged line leaked into log:\n%s", content)
	}
	if !strings.Contains(content, "--- Log started") ||
		!strings.Contains(content, "--- Log stopped") {
		t.Errorf("expected start/stop stamps, got:\n%s", content)
	}
}

// TestLogDefaultPathUnderConfigDir verifies /log start with no
// argument creates a timestamped file under <config>/logs/.
func TestLogDefaultPathUnderConfigDir(t *testing.T) {
	s, _, _ := newTestSession(t)

	userInput(s, "/log start")
	path, active := s.LogStatus()
	if !active {
		t.Fatal("log not active after /log start")
	}
	wantPrefix := filepath.Join(s.config.ConfigDir, "logs") + string(filepath.Separator)
	if !strings.HasPrefix(path, wantPrefix) {
		t.Errorf("default log path = %q, want under %q", path, wantPrefix)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("default log file not created: %v", err)
	}
}

// TestLogSurvivesReload verifies the Go-owned file handle keeps
// logging across the Lua VM teardown of /reload.
func TestLogSurvivesReload(t *testing.T) {
	s, _, _ := newTestSession(t)

	path := filepath.Join(s.config.ConfigDir, "reload.log")
	userInput(s, "/log start "+path)

	s.Reload()
	ev := <-s.events // reload is deferred via AsyncResult
	s.handleEvent(ev)

	if got, active := s.LogStatus(); !active || got != path {
		t.Fatalf("log did not survive reload: path=%q active=%v", got, active)
	}

	serverLine(s, "after reload")
	userInput(s, "/log stop")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(data), "after reload") {
		t.Errorf("line after reload missing from log:\n%s", data)
	}
}
