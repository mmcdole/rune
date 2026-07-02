package ui

// BarContent holds the rendered content of a bar.
type BarContent struct {
	Left   string
	Center string
	Right  string
}

// PickerItem represents an item for picker/selection UI.
type PickerItem struct {
	Text        string
	Description string
	Value       string // ID or Value passed back to caller
	MatchDesc   bool   // If true, include Description in fuzzy matching
}

// FilterValue returns the string used for fuzzy matching.
func (p PickerItem) FilterValue() string {
	if p.MatchDesc && p.Description != "" {
		return p.Text + " " + p.Description
	}
	return p.Text
}

// GetText returns the item's display text.
func (p PickerItem) GetText() string { return p.Text }

// GetDescription returns the item's description.
func (p PickerItem) GetDescription() string { return p.Description }

// GetValue returns the item's value (returned on selection).
func (p PickerItem) GetValue() string { return p.Value }

// MatchesDescription returns true if description should be included in matching.
func (p PickerItem) MatchesDescription() bool { return p.MatchDesc }
