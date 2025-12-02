package picker

// Item represents a row in the picker.
// Implement this interface for custom item types.
type Item interface {
	// FilterValue returns the string to be used for fuzzy matching.
	FilterValue() string

	// GetText returns the item's display text.
	GetText() string

	// GetDescription returns the item's description (may be empty).
	GetDescription() string

	// GetValue returns the value to return on selection.
	GetValue() string

	// MatchesDescription returns true if description is included in fuzzy matching.
	MatchesDescription() bool
}
