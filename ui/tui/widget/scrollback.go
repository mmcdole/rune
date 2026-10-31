package widget

// Scrollback is a ring buffer of logical lines of terminal output. Lines
// are stored unwrapped; Output wraps them to its current width on demand.
type Scrollback struct {
	lines    []string
	rows     []rowMemo // per-slot wrapped row count, valid only at its width
	head     int
	tail     int
	count    int
	capacity int
	appended uint64 // lines ever appended; assigns eviction-stable sequence numbers
}

// rowMemo caches how many rows a line wraps to at one width. A width
// change invalidates every memo implicitly, with no pass over the ring.
type rowMemo struct {
	width int32
	rows  int32
}

// NewScrollback creates a new ring buffer.
func NewScrollback(capacity int) *Scrollback {
	if capacity <= 0 {
		capacity = 100000
	}
	return &Scrollback{
		lines:    make([]string, capacity),
		rows:     make([]rowMemo, capacity),
		capacity: capacity,
	}
}

// Append adds a line to the buffer.
func (sb *Scrollback) Append(line string) {
	sb.lines[sb.tail] = line
	sb.rows[sb.tail] = rowMemo{}
	sb.tail = (sb.tail + 1) % sb.capacity
	sb.appended++

	if sb.count < sb.capacity {
		sb.count++
	} else {
		sb.head = (sb.head + 1) % sb.capacity
	}
}

// Count returns the number of lines.
func (sb *Scrollback) Count() int {
	return sb.count
}

// Clear removes all buffered lines while keeping sequence numbers monotonic so
// stale search anchors can never refer to newly appended content.
func (sb *Scrollback) Clear() {
	clear(sb.lines)
	clear(sb.rows)
	sb.head = 0
	sb.tail = 0
	sb.count = 0
}

// At retrieves a line by index (0 = oldest).
func (sb *Scrollback) At(i int) string {
	if i < 0 || i >= sb.count {
		return ""
	}
	actualIndex := (sb.head + i) % sb.capacity
	return sb.lines[actualIndex]
}

// rowCount returns how many rows the line at index i wraps to at width,
// computing it with measure only on a memo miss.
func (sb *Scrollback) rowCount(i, width int, measure func(string, int) int) int {
	slot := (sb.head + i) % sb.capacity
	if memo := sb.rows[slot]; memo.rows > 0 && int(memo.width) == width {
		return int(memo.rows)
	}
	n := measure(sb.lines[slot], width)
	sb.rows[slot] = rowMemo{width: int32(width), rows: int32(n)}
	return n
}

// Seq returns the absolute sequence number of the line at index i.
// Indices shift as the full ring evicts old lines; sequence numbers
// never do, so they can anchor positions across appends.
func (sb *Scrollback) Seq(i int) uint64 {
	return sb.appended - uint64(sb.count) + uint64(i)
}

// IndexOf maps an absolute sequence number back to a current index;
// ok is false when that line has been evicted (or never existed).
func (sb *Scrollback) IndexOf(seq uint64) (int, bool) {
	oldest := sb.appended - uint64(sb.count)
	if seq < oldest || seq >= sb.appended {
		return 0, false
	}
	return int(seq - oldest), true
}
