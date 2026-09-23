package ui

// BarContent holds the rendered content of a bar.
type BarContent struct {
	Left   string
	Center string
	Right  string
}

// PickerItem represents an item for picker/selection UI. Text and Description
// are unstyled presentation text and are made terminal-safe by the renderer;
// Value is an opaque payload returned byte-for-byte on selection.
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

// Config carries the UI-facing subset of Rune's typed configuration.
type Config struct {
	// KeepInput keeps a submitted command selected in the input line, so Enter
	// resends it and typing replaces it.
	KeepInput bool
	// Numpad enables terminal modes that preserve physical numpad keys. Rune
	// still accepts an already-distinct numpad event when this is false.
	Numpad bool
	// Mouse captures terminal mouse events so the wheel can scroll Rune's
	// viewport. When disabled, the terminal retains native text selection.
	Mouse bool
}

// PickerOptions configures a picker overlay opened by UI.ShowPicker.
type PickerOptions struct {
	Title      string       // Optional title/header for the picker (modal mode only)
	Items      []PickerItem // Items to display
	CallbackID string       // Opaque ID to track which Lua callback to run
	// Inline mode: picker filters based on input content, doesn't trap keys.
	// Modal mode (default): picker captures keyboard and has its own search field.
	Inline bool
	// DismissOnSpace closes an inline picker as soon as the input contains
	// a space - for pickers over single-token items (slash commands) where
	// a space means the user has committed and is typing arguments.
	DismissOnSpace bool
}

// SearchOptions configures the scrollback-search overlay opened by UI.ShowSearch.
// Search is self-contained in the UI; interaction and viewport changes are
// reported through UIEvent.
type SearchOptions struct {
	Query string // initial query; empty keeps the previous search's query
}
