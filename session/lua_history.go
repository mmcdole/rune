package session

import "github.com/mmcdole/rune/input"

// GetHistory implements lua.Host.
// It is the compatibility projection used by rune.history.get().
func (s *Session) GetHistory() []string {
	result := make([]string, len(s.historyEntries))
	for i, entry := range s.historyEntries {
		result[i] = entry.Text
	}
	return result
}

// GetHistoryEntries implements lua.Host and preserves submission mode.
func (s *Session) GetHistoryEntries() []input.Submission {
	result := make([]input.Submission, len(s.historyEntries))
	copy(result, s.historyEntries)
	return result
}

// AddToHistory implements lua.Host.
// The legacy Lua API adds ordinary command entries.
func (s *Session) AddToHistory(cmd string) {
	s.addHistorySubmission(input.Command(cmd))
}

// addHistorySubmission records the immutable submission as one history item.
// Adjacent entries dedupe only when both text and interpretation match.
func (s *Session) addHistorySubmission(entry input.Submission) {
	if entry.Text == "" {
		return
	}
	if len(s.historyEntries) > 0 && s.historyEntries[len(s.historyEntries)-1] == entry {
		return
	}
	s.historyEntries = append(s.historyEntries, entry)
	if len(s.historyEntries) > s.historyLimit {
		s.historyEntries = s.historyEntries[len(s.historyEntries)-s.historyLimit:]
	}
}
