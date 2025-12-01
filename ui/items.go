package ui

import (
	"strings"

	"github.com/drake/rune/ui/style"
)

// GenericItem is a general purpose picker item for Lua-driven lists.
type GenericItem struct {
	Text        string
	Description string
	Value       string // ID or Value passed back to Lua
	MatchDesc   bool   // If true, include Description in fuzzy matching
}

func (i GenericItem) FilterValue() string {
	if i.MatchDesc && i.Description != "" {
		return i.Text + " " + i.Description
	}
	return i.Text
}

func (i GenericItem) Render(width int, selected bool, matches []int, s style.Styles) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	// Create match set for O(1) lookup
	matchSet := make(map[int]bool, len(matches))
	for _, pos := range matches {
		matchSet[pos] = true
	}

	// Build highlighted text
	var result strings.Builder

	// Render Text portion with highlights (positions 0..len(Text)-1)
	for idx, r := range i.Text {
		ch := string(r)
		if matchSet[idx] && selected {
			result.WriteString(s.OverlayMatchSelected.Render(ch))
		} else if matchSet[idx] {
			result.WriteString(s.OverlayMatch.Render(ch))
		} else if selected {
			result.WriteString(s.OverlaySelected.Render(ch))
		} else {
			result.WriteString(s.OverlayNormal.Render(ch))
		}
	}

	// Add separator and Description if present
	if i.Description != "" {
		sep := " - "
		if selected {
			result.WriteString(s.OverlaySelected.Render(sep))
		} else {
			result.WriteString(s.OverlayNormal.Render(sep))
		}

		// Description highlight positions depend on MatchDesc flag
		// If MatchDesc: positions len(Text)+1.. correspond to Description
		// If !MatchDesc: no matches in Description (it wasn't searched)
		descOffset := len(i.Text) + 1
		for idx, r := range i.Description {
			ch := string(r)
			isMatch := i.MatchDesc && matchSet[descOffset+idx]
			if isMatch && selected {
				result.WriteString(s.OverlayMatchSelected.Render(ch))
			} else if isMatch {
				result.WriteString(s.OverlayMatch.Render(ch))
			} else if selected {
				result.WriteString(s.OverlaySelected.Render(ch))
			} else {
				result.WriteString(s.OverlayNormal.Render(ch))
			}
		}
	}

	// Apply prefix styling
	var prefixStyled string
	if selected {
		prefixStyled = s.OverlaySelected.Render(prefix)
	} else {
		prefixStyled = s.OverlayNormal.Render(prefix)
	}

	return prefixStyled + result.String()
}
