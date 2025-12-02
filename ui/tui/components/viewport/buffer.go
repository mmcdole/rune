package viewport

// ScrollbackBuffer is a ring buffer for storing terminal output lines.
// It provides O(1) append, O(1) eviction when full, and O(1) random access.
type ScrollbackBuffer struct {
	lines    []string // Fixed-size ring buffer
	head     int      // Index of oldest line
	tail     int      // Index where next line will be written
	count    int      // Current number of lines
	capacity int      // Maximum number of lines
}

// NewScrollbackBuffer creates a new ring buffer with the given capacity.
func NewScrollbackBuffer(capacity int) *ScrollbackBuffer {
	if capacity <= 0 {
		capacity = 100000 // Default 100k lines
	}
	return &ScrollbackBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Append adds a line to the buffer. If full, the oldest line is evicted.
func (sb *ScrollbackBuffer) Append(line string) {
	sb.lines[sb.tail] = line
	sb.tail = (sb.tail + 1) % sb.capacity

	if sb.count < sb.capacity {
		sb.count++
	} else {
		// Buffer is full, advance head to evict oldest
		sb.head = (sb.head + 1) % sb.capacity
	}
}

// AppendBatch adds multiple lines efficiently.
func (sb *ScrollbackBuffer) AppendBatch(lines []string) {
	for _, line := range lines {
		sb.Append(line)
	}
}

// Count returns the number of lines currently in the buffer.
func (sb *ScrollbackBuffer) Count() int {
	return sb.count
}

// Capacity returns the maximum number of lines the buffer can hold.
func (sb *ScrollbackBuffer) Capacity() int {
	return sb.capacity
}

// Get retrieves a line by logical index (0 = oldest, count-1 = newest).
// Returns empty string if index is out of bounds.
func (sb *ScrollbackBuffer) Get(index int) string {
	if index < 0 || index >= sb.count {
		return ""
	}
	actualIndex := (sb.head + index) % sb.capacity
	return sb.lines[actualIndex]
}

// At retrieves a line by logical index without bounds checking allocations.
// This is a zero-allocation accessor for use in tight render loops.
// Returns empty string if index is out of bounds.
func (sb *ScrollbackBuffer) At(i int) string {
	if i < 0 || i >= sb.count {
		return ""
	}
	actualIndex := (sb.head + i) % sb.capacity
	return sb.lines[actualIndex]
}

// GetRange retrieves a slice of lines by logical index range [start, end).
// Handles bounds automatically.
func (sb *ScrollbackBuffer) GetRange(start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > sb.count {
		end = sb.count
	}
	if start >= end {
		return nil
	}

	result := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		result = append(result, sb.Get(i))
	}
	return result
}

// GetNewest retrieves the n most recent lines (newest last).
func (sb *ScrollbackBuffer) GetNewest(n int) []string {
	if n <= 0 {
		return nil
	}
	if n > sb.count {
		n = sb.count
	}
	return sb.GetRange(sb.count-n, sb.count)
}

// Clear removes all lines from the buffer.
func (sb *ScrollbackBuffer) Clear() {
	sb.head = 0
	sb.tail = 0
	sb.count = 0
	// Note: We don't zero the backing array for performance.
	// Old data will be overwritten on next append.
}

// IsFull returns true if the buffer is at capacity.
func (sb *ScrollbackBuffer) IsFull() bool {
	return sb.count == sb.capacity
}
