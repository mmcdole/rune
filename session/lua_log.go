package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drake/rune/text"
)

// Session logging (lua.Host implementation). The file handle is
// Go-owned so an active log survives /reload; Run's defer closes it on
// exit. All methods run on the session goroutine (called from Lua), so
// no locking is needed.

// LogStart implements lua.Host. Opens path in append mode, creating
// parent directories. An already-open log is closed and replaced.
func (s *Session) LogStart(path string) (string, error) {
	path = expandHome(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if s.logFile != nil {
		s.logFile.Close()
	}
	s.logFile = f
	s.logPath = abs
	return abs, nil
}

// LogStop implements lua.Host.
func (s *Session) LogStop() bool {
	if s.logFile == nil {
		return false
	}
	s.logFile.Close()
	s.logFile = nil
	s.logPath = ""
	return true
}

// LogWrite implements lua.Host. A write failure (disk full, file
// deleted) closes the log and reports once, rather than erroring on
// every subsequent line.
func (s *Session) LogWrite(line string) {
	if s.logFile == nil {
		return
	}
	if _, err := s.logFile.WriteString(line + "\n"); err != nil {
		path := s.logPath
		s.LogStop()
		s.ui.Print(text.Red(fmt.Sprintf("[Log] write to %s failed (%v) - logging stopped", path, err)))
	}
}

// LogStatus implements lua.Host.
func (s *Session) LogStatus() (string, bool) {
	return s.logPath, s.logFile != nil
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
