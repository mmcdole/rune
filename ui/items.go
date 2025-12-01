package ui

import "github.com/drake/rune/ui/style"

// GenericItem is a general purpose picker item for Lua-driven lists.
type GenericItem struct {
	Text        string
	Description string
	Value       string // ID or Value passed back to Lua
}

func (i GenericItem) FilterValue() string {
	return i.Text + " " + i.Description
}

func (i GenericItem) Render(width int, selected bool, matches []int, s style.Styles) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	text := i.Text
	if i.Description != "" {
		text += " - " + i.Description
	}

	if selected {
		return s.OverlaySelected.Render(prefix + text)
	}
	return s.OverlayNormal.Render(prefix + text)
}
