package session

// HistoryManager manages input command history.
type HistoryManager struct {
	lines []string
	limit int
}

// NewHistoryManager creates a new history manager with the given limit.
func NewHistoryManager(limit int) *HistoryManager {
	return &HistoryManager{
		lines: make([]string, 0, limit),
		limit: limit,
	}
}

// Add appends a command to history, skipping duplicates of the last entry.
func (h *HistoryManager) Add(cmd string) {
	if cmd == "" {
		return
	}
	// Don't add duplicates of the last command
	if len(h.lines) > 0 && h.lines[len(h.lines)-1] == cmd {
		return
	}
	h.lines = append(h.lines, cmd)
	// Trim if over limit
	if len(h.lines) > h.limit {
		h.lines = h.lines[len(h.lines)-h.limit:]
	}
}

// Get returns a copy of the history.
func (h *HistoryManager) Get() []string {
	result := make([]string, len(h.lines))
	copy(result, h.lines)
	return result
}
