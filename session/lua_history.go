package session

// GetHistory implements lua.HistoryService.
func (s *Session) GetHistory() []string {
	return s.history.Get()
}

// AddToHistory implements lua.HistoryService.
func (s *Session) AddToHistory(cmd string) {
	s.history.Add(cmd)
}
