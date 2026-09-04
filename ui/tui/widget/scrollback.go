package widget

// ScrollbackBuffer is a ring buffer of physical rows of terminal
// output; each entry renders as exactly one row.
type ScrollbackBuffer struct {
	lines    []string
	head     int
	tail     int
	count    int
	capacity int
	appended uint64 // rows ever appended; assigns eviction-stable sequence numbers
}

// NewScrollbackBuffer creates a new ring buffer.
func NewScrollbackBuffer(capacity int) *ScrollbackBuffer {
	if capacity <= 0 {
		capacity = 100000
	}
	return &ScrollbackBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Append adds a row to the buffer.
func (sb *ScrollbackBuffer) Append(row string) {
	sb.lines[sb.tail] = row
	sb.tail = (sb.tail + 1) % sb.capacity
	sb.appended++

	if sb.count < sb.capacity {
		sb.count++
	} else {
		sb.head = (sb.head + 1) % sb.capacity
	}
}

// Count returns the number of rows.
func (sb *ScrollbackBuffer) Count() int {
	return sb.count
}

// Clear removes all buffered rows while keeping sequence numbers monotonic so
// stale search anchors can never refer to newly appended content.
func (sb *ScrollbackBuffer) Clear() {
	clear(sb.lines)
	sb.head = 0
	sb.tail = 0
	sb.count = 0
}

// At retrieves a row by index (0 = oldest).
func (sb *ScrollbackBuffer) At(i int) string {
	if i < 0 || i >= sb.count {
		return ""
	}
	actualIndex := (sb.head + i) % sb.capacity
	return sb.lines[actualIndex]
}

// Seq returns the absolute sequence number of the row at index i.
// Indices shift as the full ring evicts old rows; sequence numbers
// never do, so they can anchor positions across appends.
func (sb *ScrollbackBuffer) Seq(i int) uint64 {
	return sb.appended - uint64(sb.count) + uint64(i)
}

// IndexOf maps an absolute sequence number back to a current index;
// ok is false when that row has been evicted (or never existed).
func (sb *ScrollbackBuffer) IndexOf(seq uint64) (int, bool) {
	oldest := sb.appended - uint64(sb.count)
	if seq < oldest || seq >= sb.appended {
		return 0, false
	}
	return int(seq - oldest), true
}
