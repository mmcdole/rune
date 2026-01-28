package session

// GetHistory implements lua.Host.
func (s *Session) GetHistory() []string {
	result := make([]string, len(s.historyLines))
	copy(result, s.historyLines)
	return result
}

// AddToHistory implements lua.Host.
func (s *Session) AddToHistory(cmd string) {
	if cmd == "" {
		return
	}
	// Don't add duplicates of the last command
	if len(s.historyLines) > 0 && s.historyLines[len(s.historyLines)-1] == cmd {
		return
	}
	s.historyLines = append(s.historyLines, cmd)
	// Trim if over limit
	if len(s.historyLines) > s.historyLimit {
		s.historyLines = s.historyLines[len(s.historyLines)-s.historyLimit:]
	}
}
