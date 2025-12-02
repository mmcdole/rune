package util

import (
	"strings"
	"sync"
	"unicode"

	"github.com/drake/rune/text"
)

// CompletionEngine maintains unique words in LRU order for tab completion.
// Words are extracted from server output and stored with most recent last.
// Thread-safe for concurrent access.
type CompletionEngine struct {
	mu       sync.RWMutex
	words    []string       // LRU ordered: oldest first, newest last
	index    map[string]int // word -> position for O(1) lookup
	capacity int
}

// NewCompletionEngine creates a completion engine with the given capacity.
func NewCompletionEngine(capacity int) *CompletionEngine {
	return &CompletionEngine{
		words:    make([]string, 0, capacity),
		index:    make(map[string]int),
		capacity: capacity,
	}
}

// AddLine extracts words from a server line and adds them to the cache.
// Words are tokenized and punctuation is stripped.
func (ce *CompletionEngine) AddLine(line string) {
	// Strip ANSI codes first
	clean := text.StripANSI(line)

	// Tokenize on non-letter/digit boundaries
	tokens := strings.FieldsFunc(clean, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	ce.mu.Lock()
	defer ce.mu.Unlock()

	for _, token := range tokens {
		// Skip short words (noise like "a", "to", "is")
		if len(token) < 3 {
			continue
		}
		ce.addWordLocked(token)
	}
}

// AddInput adds words from user input, preserving punctuation.
// Each space-separated token is added as-is (for aliases like "blah!").
func (ce *CompletionEngine) AddInput(input string) {
	tokens := strings.Fields(input)

	ce.mu.Lock()
	defer ce.mu.Unlock()

	for _, token := range tokens {
		if len(token) < 2 {
			continue
		}
		ce.addWordLocked(token)
	}
}

// addWordLocked adds a single word to the cache, moving it to end if it exists.
// Words are stored lowercase for consistent matching.
// Caller must hold the lock.
func (ce *CompletionEngine) addWordLocked(word string) {
	word = strings.ToLower(word)
	if pos, exists := ce.index[word]; exists {
		// Remove from current position
		ce.words = append(ce.words[:pos], ce.words[pos+1:]...)
		// Update indices for shifted words
		for i := pos; i < len(ce.words); i++ {
			ce.index[ce.words[i]] = i
		}
	}

	// Append to end (most recent)
	ce.words = append(ce.words, word)
	ce.index[word] = len(ce.words) - 1

	// Trim if over capacity
	if len(ce.words) > ce.capacity {
		oldest := ce.words[0]
		delete(ce.index, oldest)
		ce.words = ce.words[1:]
		// Update all indices
		for i, w := range ce.words {
			ce.index[w] = i
		}
	}
}

// FindMatches returns words matching the prefix, newest first.
// All words are stored and returned lowercase.
func (ce *CompletionEngine) FindMatches(prefix string) []string {
	if prefix == "" {
		return nil
	}

	prefixLower := strings.ToLower(prefix)

	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var matches []string

	// Iterate backwards (newest first)
	for i := len(ce.words) - 1; i >= 0; i-- {
		word := ce.words[i]
		if strings.HasPrefix(word, prefixLower) {
			matches = append(matches, word)
		}
	}

	return matches
}
