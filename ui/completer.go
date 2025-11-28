package ui

import (
	"strings"
	"unicode"
)

// WordCache maintains unique words in LRU order for tab completion.
// Words are extracted from server output and stored with most recent last.
type WordCache struct {
	words    []string       // LRU ordered: oldest first, newest last
	index    map[string]int // word -> position for O(1) lookup
	capacity int
}

// NewWordCache creates a word cache with the given capacity.
func NewWordCache(capacity int) *WordCache {
	return &WordCache{
		words:    make([]string, 0, capacity),
		index:    make(map[string]int),
		capacity: capacity,
	}
}

// AddLine extracts words from a server line and adds them to the cache.
// Words are tokenized and punctuation is stripped.
func (wc *WordCache) AddLine(line string) {
	// Strip ANSI codes first
	clean := stripAnsi(line)

	// Tokenize on non-letter/digit boundaries
	tokens := strings.FieldsFunc(clean, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	for _, token := range tokens {
		// Skip short words (noise like "a", "to", "is")
		if len(token) < 3 {
			continue
		}
		wc.addWord(token)
	}
}

// AddInput adds words from user input, preserving punctuation.
// Each space-separated token is added as-is (for aliases like "blah!").
func (wc *WordCache) AddInput(input string) {
	tokens := strings.Fields(input)
	for _, token := range tokens {
		if len(token) < 2 {
			continue
		}
		wc.addWord(token)
	}
}

// addWord adds a single word to the cache, moving it to end if it exists.
// Words are stored lowercase for consistent matching.
func (wc *WordCache) addWord(word string) {
	word = strings.ToLower(word)
	if pos, exists := wc.index[word]; exists {
		// Remove from current position
		wc.words = append(wc.words[:pos], wc.words[pos+1:]...)
		// Update indices for shifted words
		for i := pos; i < len(wc.words); i++ {
			wc.index[wc.words[i]] = i
		}
	}

	// Append to end (most recent)
	wc.words = append(wc.words, word)
	wc.index[word] = len(wc.words) - 1

	// Trim if over capacity
	if len(wc.words) > wc.capacity {
		oldest := wc.words[0]
		delete(wc.index, oldest)
		wc.words = wc.words[1:]
		// Update all indices
		for i, w := range wc.words {
			wc.index[w] = i
		}
	}
}

// FindMatches returns words matching the prefix, newest first.
// All words are stored and returned lowercase.
func (wc *WordCache) FindMatches(prefix string) []string {
	if prefix == "" {
		return nil
	}

	prefixLower := strings.ToLower(prefix)
	var matches []string

	// Iterate backwards (newest first)
	for i := len(wc.words) - 1; i >= 0; i-- {
		word := wc.words[i]
		if strings.HasPrefix(word, prefixLower) {
			matches = append(matches, word)
		}
	}

	return matches
}
