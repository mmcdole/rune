package tui

import "strings"

// InputComponent adapts the input area for the Component interface.
// It includes the picker overlay, borders, and input field.
type InputComponent struct {
	m *Model
}

// Height returns the total height of the input area.
func (c *InputComponent) Height() int {
	h := 3 // top border + input + bottom border
	if c.m.pickerActive() {
		h += c.m.picker.Height()
	}
	return h
}

// Render returns the input area with optional picker overlay.
func (c *InputComponent) Render(width int) string {
	var parts []string

	// Picker overlay (if active)
	if c.m.pickerActive() {
		parts = append(parts, c.m.picker.View())
	}

	// Top border
	parts = append(parts, c.m.borderLine())

	// Input field
	parts = append(parts, c.m.input.View())

	// Bottom border
	parts = append(parts, c.m.borderLine())

	return strings.Join(parts, "\n")
}
