package util

import (
	"sort"
	"strings"
	"unicode"
)

// Match represents a scored fuzzy match result.
type Match struct {
	Index     int    // Original index in input slice
	Text      string // The matched text
	Score     int    // Match quality (higher = better)
	Positions []int  // Matched character positions (for highlighting)
}

// FuzzyFilter filters and ranks items by fuzzy match quality against pattern.
// Returns matches sorted by score (best first). Empty pattern returns all items.
//
// Pattern is split on spaces - each term must match (AND logic), but order doesn't matter.
// This matches fzf behavior: "test this" matches "this is a test".
func FuzzyFilter(pattern string, items []string) []Match {
	if pattern == "" {
		// No pattern - return all items with zero score, original order
		matches := make([]Match, len(items))
		for i, item := range items {
			matches[i] = Match{Index: i, Text: item, Score: 0}
		}
		return matches
	}

	// Split pattern into terms (fzf-style: space separates AND terms)
	terms := strings.Fields(pattern)

	var matches []Match
	for i, item := range items {
		score, positions := fuzzyScoreMulti(terms, item)
		if score > 0 {
			matches = append(matches, Match{
				Index:     i,
				Text:      item,
				Score:     score,
				Positions: positions,
			})
		}
	}

	// Sort by score descending, then by original index for stability
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Index < matches[j].Index
	})

	return matches
}

// fuzzyScoreMulti scores text against multiple terms (AND logic).
// All terms must match for a positive score.
func fuzzyScoreMulti(terms []string, text string) (int, []int) {
	if len(terms) == 0 {
		return 0, nil
	}

	// Single term - use regular scoring
	if len(terms) == 1 {
		return FuzzyScore(terms[0], text)
	}

	// Multiple terms - all must match
	totalScore := 0
	var allPositions []int
	positionSet := make(map[int]bool) // Avoid duplicate positions

	for _, term := range terms {
		score, positions := FuzzyScore(term, text)
		if score == 0 {
			return 0, nil // Term didn't match - fail
		}
		totalScore += score
		for _, p := range positions {
			if !positionSet[p] {
				positionSet[p] = true
				allPositions = append(allPositions, p)
			}
		}
	}

	// Sort positions for consistent highlighting
	sort.Ints(allPositions)

	return totalScore, allPositions
}

// FuzzyScore computes a fuzzy match score for pattern against text.
// Returns (score, positions). Score of 0 means no match.
// Higher scores indicate better matches.
//
// Uses fzf-style algorithm: forward scan to verify match exists,
// then backward scan to find the tightest cluster of matches.
func FuzzyScore(pattern, text string) (int, []int) {
	if pattern == "" || text == "" {
		return 0, nil
	}

	patternLower := strings.ToLower(pattern)
	textLower := strings.ToLower(text)
	textRunes := []rune(text)
	textLowerRunes := []rune(textLower)
	patternRunes := []rune(patternLower)

	// Phase 1: Forward scan - verify all pattern chars exist in order
	// Find the LAST position where full pattern can match
	pIdx := 0
	endIdx := -1
	for i := 0; i < len(textLowerRunes) && pIdx < len(patternRunes); i++ {
		if textLowerRunes[i] == patternRunes[pIdx] {
			endIdx = i
			pIdx++
		}
	}

	// Pattern didn't fully match
	if pIdx < len(patternRunes) {
		return 0, nil
	}

	// Phase 2: Backward scan - find tightest match ending at endIdx
	// This finds the shortest substring containing all pattern chars
	positions := make([]int, len(patternRunes))
	pIdx = len(patternRunes) - 1
	for i := endIdx; i >= 0 && pIdx >= 0; i-- {
		if textLowerRunes[i] == patternRunes[pIdx] {
			positions[pIdx] = i
			pIdx--
		}
	}

	// Phase 3: Score the match
	score := 0
	firstPos := positions[0]

	// Big bonus for match starting early in the string
	// This heavily favors command name matches over description matches
	score += max(0, 50-firstPos*3)

	for i, pos := range positions {
		// Boundary bonuses
		if pos == 0 {
			score += 16 // Start of string
		} else {
			prevChar := textRunes[pos-1]
			if prevChar == ' ' || prevChar == '/' || prevChar == '_' || prevChar == '-' || prevChar == '.' {
				score += 8 // Word boundary
			} else if unicode.IsLower(prevChar) && unicode.IsUpper(textRunes[pos]) {
				score += 7 // CamelCase boundary
			}
		}

		// Consecutive match bonus
		if i > 0 && positions[i] == positions[i-1]+1 {
			score += 8
		}

		// Gap penalty
		if i > 0 && positions[i] > positions[i-1]+1 {
			gap := positions[i] - positions[i-1] - 1
			score -= 3 + gap // Gap start + extension
		}
	}

	// Ensure minimum score of 1 for valid matches
	if score <= 0 {
		score = 1
	}

	return score, positions
}
